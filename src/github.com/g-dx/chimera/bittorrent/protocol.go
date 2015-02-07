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

type ProtocolHandler struct {
	metaInfo         *MetaInfo
	peers            []*Peer
	trackerResponses <-chan *TrackerResponse
	newPeers         chan *Peer
	pieceMap         *PieceMap
	done             chan struct{}
	dir              string
	logger           *log.Logger
	peerMsgs         chan *MessageList
	peerErrors       chan PeerError
	isSeed           bool
	requestTimer     *ProtocolRequestTimer
	disk             *Disk
	diskOps          chan DiskOpResult
}

func NewProtocolHandler(mi *MetaInfo, dir string, tr <-chan *TrackerResponse) (*ProtocolHandler, error) {

	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/protocol.log", dir))
	if err != nil {
		return nil, err
	}
	logger := log.New(f, "", log.Ldate|log.Ltime)

	// Create disk files
	files, err := CreateOrRead(mi.Files, fmt.Sprintf("%v/files", dir))
	if err != nil {
		return nil, err
	}

	// Create disk
	ops := make(chan DiskOpResult)
	layout := NewDiskLayout(files, mi.PieceLength, mi.TotalLength())
	fileio := NewFileIO(layout, logger)
	cacheio := NewCacheIO(layout, mi.Hashes, _16KB, ops, fileio)
	disk := NewDisk(cacheio, ops)

	// Create piece map
	pieceMap := NewPieceMap(len(mi.Hashes), int(mi.PieceLength), mi.TotalLength())

	// Create coordinator
	ph := &ProtocolHandler{
		metaInfo:         mi,
		peers:            make([]*Peer, 0, idealPeers),
		trackerResponses: tr,
		newPeers:         make(chan *Peer),
		pieceMap:         pieceMap,
		done:             make(chan struct{}),
		dir:              dir,
		logger:           logger,
		peerMsgs:         make(chan *MessageList, 100),
		peerErrors:       make(chan PeerError),
		isSeed:           false,
		disk:             disk,
		diskOps:          ops,
	}

	// Start loop & return
	go ph.loop()
	return ph, nil
}

func (ph *ProtocolHandler) AwaitDone() {
	// Await a receive to say we are finished...
	<-ph.done
}

func (ph *ProtocolHandler) Close() {
	// TODO: !!!
}

func (ph *ProtocolHandler) loop() {

	tick := 0
	for {

		select {
		case <-time.After(oneSecond):
			ph.onTick(tick)
			tick++

		case r := <-ph.trackerResponses:
			ph.onTrackerResponse(r)

		case list := <-ph.peerMsgs:
			p := ph.findPeer(list.id)
			if p != nil {
				// Process all messages
				err, net, disk := OnMessages(list.msgs, p)
				if err != nil {
					ph.closePeer(p, err)
				}
				// Send to net
				for msg := range net {
					p.queue.Add(msg)
				}
				// Send to disk
				for msg := range disk {
					ph.disk.in <- msg
				}
			}

		case e := <-ph.peerErrors:
			p := ph.findPeer(e.id)
			if p != nil {
				ph.closePeer(p, e.err)
			}

		case p := <-ph.newPeers:
			ph.peers = append(ph.peers, p)

		case d := <-ph.diskOps:
			ph.onDisk(d)
		}
	}
}

func (ph *ProtocolHandler) onDisk(op DiskOpResult) {
	switch r := op.(type) {
	case ReadOk:
		p := ph.findPeer(r.id)
		if p != nil {
			p.Add(r.block)
			ph.logger.Printf("block [%v, %v] added to peer Q [%v]\n", r.block.index, r.block.begin, r.id)
		}
	case WriteOk:
		// Nothing to do
	case PieceOk:
		ph.logger.Printf("piece [%v] written to disk\n", r)

		// Mark piece as complete
		ph.pieceMap.Get(int(r)).Complete()

		// Send haves
		for _, p := range ph.peers {
			p.Add(Have(r))
		}

		// Are we complete?
		if ph.pieceMap.IsComplete() {
			close(ph.done) // Yay!
		}
	case HashFailed:
		// Reset piece
		ph.pieceMap.Get(int(r)).Reset()

	case ErrorResult:
		ph.logger.Panicf("Disk [%v] Failed: %v\n", r.op, r.err)
	}
}

func (ph *ProtocolHandler) onTrackerResponse(r *TrackerResponse) {

	peerCount := len(ph.peers)
	if peerCount < idealPeers {

		// Add some
		for _, pa := range r.PeerAddresses[25:] {
			go ph.handlePeerConnect(pa)
			peerCount++
			if peerCount == idealPeers {
				break
			}
		}
	}
}

func (ph *ProtocolHandler) handlePeerConnect(addr PeerAddress) {

	conn, err := NewConnection(addr.GetIpAndPort())
	if err != nil {
		ph.logger.Printf("Can't connect to [%v]: %v\n", addr, err)
		return
	}

	// Create log file & create loggers
	path := fmt.Sprintf("%v/%v.log", ph.dir, conn.in.conn.RemoteAddr())
	file, err := os.Create(path)
	if err != nil {
		ph.logger.Printf("Can't create log file [%v]\n", path)
		conn.Close()
		return
	}

	out := make(chan ProtocolMessage)

	// Attempt to establish connection
	id, err := conn.Establish(out, ph.peerMsgs, ph.peerErrors, Handshake(ph.metaInfo.InfoHash), file, true)
	if err != nil {
		ph.logger.Printf("Can't establish connection [%v]: %v\n", addr, err)
		conn.Close()
		return
	}

	// Setup request handling
	ph.requestTimer.CreateTimer(*id)
	f := func(index, begin int) {
		ph.requestTimer.AddBlock(*id, index, begin)
	}

	// Connected
	ph.logger.Printf("New Peer: %v\n", id)
	ph.newPeers <- NewPeer(id, NewQueue(out, f), ph.pieceMap, ph.logger)
}

func (ph *ProtocolHandler) closePeer(peer *Peer, err error) {

	// TODO:

	// 1. Remove from peers
	// 2. peer.Close()
	// 3. Log errors
}

func (ph *ProtocolHandler) onTick(n int) {

	// Check for expired requests
	ph.requestTimer.Tick(n)

	// Update peer stats
	for _, p := range ph.peers {
		p.Stats().Update()
	}

	// Run choking algorithm
	if n%chokeInterval == 0 {
		old, new, chokes, unchokes := ChokePeers(ph.isSeed, ph.peers, n%optimisticChokeInterval == 0)
		// Clear optimistic
		if old != nil {
			old.state.ws = old.state.ws.NotOptimistic()
		}
		// Set optimistic
		if new != nil {
			new.state.ws = new.state.ws.Optimistic()
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
	PickPieces(ph.peers, ph.pieceMap, ph.requestTimer)
}


func (ph *ProtocolHandler) maybeConnect(r chan<- PeerConnectResult) {

	peerCount := len(ph.peers)
	if peerCount < idealPeers {
		// TODO: fix me!
	}
}

func (ph *ProtocolHandler) findPeer(id *PeerIdentity) *Peer {
	for _, p := range ph.peers {
		if p.Id().Equals(id) {
			return p
		}
	}
	return nil
}
