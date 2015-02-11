package bittorrent

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	FIFTY_MILLISECONDS = 50 * time.Millisecond
	oneSecond = 1 * time.Second
)

const (
	chokeInterval           = 10
	optimisticChokeInterval = 30
	idealPeers              = 25
)

type PeerConnectResult struct {
	peer *Peer
	err  error
	ok   bool
}

func StartProtocol(mi *MetaInfo, dir string) (error, chan struct{}, chan struct{}) {

	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/protocol.log", dir))
	if err != nil {
		return err, nil, nil
	}
	logger := log.New(f, "", log.Ldate|log.Ltime)

	// Create disk files
	files, err := CreateOrRead(mi.Files, fmt.Sprintf("%v/files", dir))
	if err != nil {
		return err, nil, nil
	}

	// Create disk
	ops := make(chan DiskMessageResult)
	layout := NewDiskLayout(files, mi.PieceLength, mi.TotalLength())
	cacheio := NewCacheIO(layout, mi.Hashes, _16KB, ops, NewDiskIO(layout, logger))
	disk := NewDisk(cacheio, ops)

	// Create piece map
	pieceMap := NewPieceMap(len(mi.Hashes), int(mi.PieceLength), mi.TotalLength())

	// Channels to notify callers
	done := make(chan struct{})
	stop := make(chan struct{})

	// Start connection listener
	cl, err := NewConnectionListener(make(chan *PeerConnection), "localhost:60001") // TODO: Externalise
	if err != nil {
		return err, nil, nil
	}

	// Start loop & return
	go protocolLoop(dir, pieceMap, disk, ops)
	return err, done, stop
}

// TODO: Probably replace many of these vars with a ProtocolConfiguration struct containing:
// - logger
// - channel sizes
// - output dir
// - choker function
// - picker function
// - tracker query interval
// - port to listen on
func protocolLoop(dir string, pieceMap *PieceMap, d *Disk, diskOps chan DiskMessageResult, logger *log.Logger, listener *ConnectionListener) {

	peers := make([]*Peer, 0, idealPeers)
	newPeers := make(chan *Peer)
	peerMsgs := make(chan *MessageList, 100)
	peerErrors := make(chan PeerError)
	isSeed := pieceMap.IsComplete()
	tick := 0

	trackerResponses := startTracker() // TODO: require tracker URLs from metainfo

	for {

		select {
		case <-time.After(oneSecond):
			onTick(tick, peers, pieceMap, isSeed)
			tick++

		case r := <-trackerResponses:
			addrs := onTrackerResponse(r, len(peers))
			for _, pa := range addrs {
				go handlePeerConnect(pa, dir, logger, peerMsgs, peerErrors, newPeers, len(pieceMap.pieces))
			}

		case list := <-peerMsgs:
			p := findPeer(list.id, peers)
			if p != nil {
				// Process all messages
				err, net, disk, blocks := OnReceiveMessages(list.msgs, p, pieceMap)
				if err != nil {
					closePeer(p, err)
					continue
				}
				// Send to net
				for _, msg := range net {
					p.queue.Add(msg)
				}
				// Send to disk
				for _, msg := range disk {
					disk.in <- msg
				}
				// Return blocks to piecemap
				for _, _ = range blocks {
//					ph.pieceMap.ReturnBlock() // TODO: Calculate index & begin
				}
			}

		case e := <-peerErrors:
			p := findPeer(e.id, peers)
			if p != nil {
				closePeer(p, e.err)
			}

		case p := <-newPeers:
			peers = append(peers, p)

		case d := <-diskOps:
			onDisk(d, peers, logger, pieceMap, done)

		case c := <-listener.conns:
			if len(peers) < idealPeers {
				go handlePeerEstablish(c, dir, logger, peerMsgs, peerErrors, newPeers, len(pieceMap.pieces), false)
			}
		}
	}
}

// TODO: This function should be broken down into onWriteOk(...), onReadOk(...) and the switch on type moved to the main loop
func onDisk(op DiskMessageResult, peers []*Peer, logger *log.Logger, pieceMap *PieceMap, done chan struct{}) {
	switch r := op.(type) {
	case ReadOk:
		p := findPeer(r.id, peers)
		if p != nil {
			p.Add(r.block)
			logger.Printf("block [%v, %v] added to peer Q [%v]\n", r.block.index, r.block.begin, r.id)
		}
	case WriteOk:
		// Nothing to do
	case PieceOk:
		logger.Printf("piece [%v] written to disk\n", r)

		// Mark piece as complete
		pieceMap.Get(int(r)).Complete()

		// Send haves
		for _, p := range peers {
			p.Add(Have(r))
		}

		// Are we complete?
		if pieceMap.IsComplete() {
			close(done) // Yay!
		}
	case HashFailed:
		// Reset piece
		pieceMap.Get(int(r)).Reset()

	case ErrorResult:
		logger.Panicf("Disk [%v] Failed: %v\n", r.op, r.err)
	}
}

func onTrackerResponse(r *TrackerResponse, peerCount int) []PeerAddress {

	var addrs []PeerAddress
	if peerCount < idealPeers {
		for _, pa := range r.PeerAddresses[25:] {
			addrs = append(addrs, pa)
			peerCount++
			if peerCount == idealPeers {
				break
			}
		}
	}
	return addrs
}

func handlePeerConnect(addr PeerAddress, dir string, logger *log.Logger, peerMsgs chan *MessageList,
	peerErrors chan<-PeerError, newPeers chan *Peer, noOfPieces int) {

	conn, err := NewConnection(addr.GetIpAndPort())
	if err != nil {
		logger.Printf("Can't connect to [%v]: %v\n", addr, err)
		return
	}
	handlePeerEstablish(conn, dir, logger, peerMsgs, peerErrors, newPeers, noOfPieces, true)
}

func handlePeerEstablish(conn *PeerConnection, dir string, logger *log.Logger, peerMsgs chan *MessageList,
	peerErrors chan<-PeerError, newPeers chan *Peer, noOfPieces int, outgoing bool) {

	// Create log file & create loggers
	path := fmt.Sprintf("%v/%v.log", dir, conn.in.conn.RemoteAddr())
	file, err := os.Create(path)
	if err != nil {
		logger.Printf("Can't create log file [%v]\n", path)
		conn.Close()
		return
	}

	out := make(chan ProtocolMessage)

	// Attempt to establish connection
	id, err := conn.Establish(out, peerMsgs, peerErrors, Handshake(ph.metaInfo.InfoHash), file, outgoing)
	if err != nil {
		logger.Printf("Can't establish connection [%v]: %v\n", addr, err)
		conn.Close()
		return
	}

	// Setup request handling
	ph.requestTimer.CreateTimer(*id)
	f := func(index, begin int) {
		ph.requestTimer.AddBlock(*id, index, begin)
	}

	// Connected
	logger.Printf("New Peer: %v\n", id)
	newPeers <- NewPeer(id, NewQueue(out, f), noOfPieces, logger)
}

func closePeer(peer *Peer, err error) {

	// TODO:

	// 1. Remove from peers
	// 2. peer.Close()
	// 3. Log errors
}

func onTick(n int, peers []*Peer, pieceMap *PieceMap, isSeed bool) {

	// Check for expired requests
//	requestTimer.Tick(n)

	// Update peer stats
	for _, p := range peers {
		p.Stats().Update()
	}

	// Run choking algorithm
	if n%chokeInterval == 0 {
		old, new, chokes, unchokes := ChokePeers(isSeed, peers, n%optimisticChokeInterval == 0)
		// Clear old optimistic
		if old != nil {
			old.ws = old.ws.NotOptimistic()
		}
		// Set new optimistic
		if new != nil {
			new.ws = new.ws.Optimistic()
		}
		// Send chokes
		for _, p := range chokes {
			p.Add(Choke{})
		}
		// Send unchokes
		for _, p := range unchokes {
			p.Add(Unchoke{})
		}
	}

	// Run piece picking algorithm
	pp := PickPieces(peers, pieceMap, requestTimer)
	for p, blocks := range pp {
		for _, msg := range blocks {
			// Send, mark as requested & add to pending map
			p.Add(msg)
			pieceMap.SetBlock(msg.index, msg.begin, REQUESTED)
			p.blocks.Add(toOffset(msg.index, msg.begin, pieceMap.pieceSize))
		}
	}
}


func maybeConnect(r chan<- PeerConnectResult) {

//	peerCount := len(ph.peers)
//	if peerCount < idealPeers {
//		// TODO: fix me!
//	}
}

func findPeer(id *PeerIdentity, peers []*Peer) *Peer {
	for _, p := range peers {
		if p.Id().Equals(id) {
			return p
		}
	}
	return nil
}

func startTracker() <-chan *TrackerResponse {
	// TODO: Implement stoppable goroutine for querying tracker
	return nil
}