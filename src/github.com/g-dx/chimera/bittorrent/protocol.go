package bittorrent

import (
	"fmt"
	"log"
	"time"
	"io"
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

// Various I/O channels for the protocol
type ProtocolIO struct {
	pNew   chan PeerWrapper
	pMsgs  chan *MessageList
	pErrs  chan PeerError
	pConns chan *PeerConnection
	dIn  chan DiskMessage
	dOut chan DiskMessageResult
	trOut chan *TrackerResponse
	tick <-chan time.Time
	complete chan struct{}
}

func NewProtocolIO(c ProtocolConfig) ProtocolIO {
	return ProtocolIO{
		make(chan PeerWrapper),
		make(chan *MessageList, c.peersIncomingBufferSize),
		make(chan PeerError),
		make(chan *PeerConnection),
		make(chan DiskMessage),
		make(chan DiskMessageResult),
		make(chan *TrackerResponse),
		time.After(oneSecond), // TODO: add to config
		make(chan struct{}),
	}
}

// TODO: Create struct for capturing configuration
// - logger
// - channel sizes
// - choker function
// - picker function
// - heartbeat frequency
type ProtocolConfig struct {
	w func(name string) (error, io.Writer)
	mi *MetaInfo
	downloadDir string
	listenAddr string
	peerOutgoingBufferSize int
	peersIncomingBufferSize int
}

type PeerWrapper struct {
	p *Peer
	in chan BufferMessage
}

func StartProtocol(c ProtocolConfig) error {

	// Create log file & create loggers
	err, f := c.w("protocol.log")
	if err != nil {
		return err
	}
	logger := log.New(f, "", log.Ldate|log.Ltime)

	// Create disk files
	files, err := CreateOrRead(c.mi.Files, c.downloadDir)
	if err != nil {
		return err
	}

	io := NewProtocolIO(c)

	// Create disk
	ops := make(chan DiskMessageResult)
	layout := NewDiskLayout(files, c.mi.PieceLength, c.mi.TotalLength())
	cacheio := NewCacheIO(layout, c.mi.Hashes, _16KB, ops, NewDiskIO(layout, logger))
	_ = NewDisk(cacheio, ops)

	// Create piece map
	pieceMap := NewPieceMap(len(c.mi.Hashes), int(c.mi.PieceLength), c.mi.TotalLength())

	// Start connection listener
	// TODO: Decide what to do here. We could wrap in a ProtocolControl struct which has
	// stop, progress, etc methods on it and return from this method?
	_, err = NewConnectionListener(io.pConns, c.listenAddr)
	if err != nil {
		return err
	}

	// Start loop & return
	go protocolLoop(c, pieceMap, io, logger)
	return err
}

func protocolLoop(c ProtocolConfig, pieceMap *PieceMap, io ProtocolIO, logger *log.Logger) {

	buffers := make(map[PeerIdentity]chan BufferMessage)
	peers := make([]*Peer, 0, idealPeers)
	isSeed := pieceMap.IsComplete()
	tick := 0

	for {

		select {
		case <-io.tick:
			OnTick(tick, peers, buffers, isSeed, pieceMap)
			tick++

		case r := <-io.trOut:
			addrs := onTrackerResponse(r, len(peers))
			for _, pa := range addrs {
				go handlePeerConnect(pa, c, logger, io)
			}

		case list := <-io.pMsgs:
			p := findPeer(list.id, peers)
			if p != nil {
				// Process all messages
				err, net, disk := OnReceiveMessages(list.msgs, p, pieceMap)
				if err != nil {
					closePeer(p, err)
					continue
				}
				// Send to net
				buffers[p.Id()] <- OnSendMessages(net, p, pieceMap)
				// Send to disk
				for _, msg := range disk {
					io.dIn <- msg
				}
			}

		case e := <-io.pErrs:
			p := findPeer(e.id, peers)
            if p != nil {
                closePeer(p, e.err)
            }

		case wrapper := <-io.pNew:
			peers = append(peers, wrapper.p)
			buffers[wrapper.p.Id()] = wrapper.in

		case d := <-io.dOut:
			onDisk(d, peers, buffers, logger, pieceMap, io.complete)

		case conn := <-io.pConns:
			if len(peers) < idealPeers {
				go handlePeerEstablish(conn, c, logger, io, false)
			}
		}
	}
}

// TODO: This function should be broken down into onWriteOk(...), onReadOk(...) and the switch on type moved to the main loop
func onDisk(op DiskMessageResult, peers []*Peer, buffers map[PeerIdentity]chan BufferMessage, logger *log.Logger, pieceMap *PieceMap, complete chan struct{}) {
	switch r := op.(type) {
	case ReadOk:
		p := findPeer(r.id, peers)
		if p != nil {
			buffers[p.Id()] <- OnSendMessages([]ProtocolMessage{ r.block }, p, pieceMap)
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
			buffers[p.Id()] <- OnSendMessages([]ProtocolMessage{ Have(r) }, p, pieceMap)
		}

		// Are we complete?
		if pieceMap.IsComplete() {
			close(complete) // Yay!
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

func handlePeerConnect(addr PeerAddress, c ProtocolConfig, logger *log.Logger, io ProtocolIO) {

	conn, err := NewConnection(addr.GetIpAndPort())
	if err != nil {
		logger.Printf("Can't connect to [%v]: %v\n", addr, err)
		return
	}
	handlePeerEstablish(conn, c, logger, io, true)
}

func handlePeerEstablish(conn *PeerConnection, c ProtocolConfig, logger *log.Logger, io ProtocolIO, outgoing bool) {

	// Create log file & create loggers
	err, file := c.w(fmt.Sprintf("%v.log", conn.in.conn.RemoteAddr()))
	if err != nil {
		logger.Printf("Can't create log file: [%v]\n", err)
		conn.Close()
		return
	}

	in, out := Buffer(c.peerOutgoingBufferSize)

	// Attempt to establish connection
	id, err := conn.Establish(out, io.pMsgs, io.pErrs, Handshake(c.mi.InfoHash), file, outgoing)
	if err != nil {
		logger.Printf("Can't establish connection [%v]: %v\n", conn.in.conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// Connected
	logger.Printf("New Peer: %v\n", id)
	io.pNew <- PeerWrapper{NewPeer(id, len(c.mi.Hashes)), in}
}

func closePeer(peer *Peer, err error) {

	// TODO:

	// 1. Remove from peers
	// 2. peer.Close()
	// 3. Log errors
}

func maybeConnect(r chan<- PeerConnectResult) {

//	peerCount := len(ph.peers)
//	if peerCount < idealPeers {
//		// TODO: fix me!
//	}
}

func findPeer(id PeerIdentity, peers []*Peer) *Peer {
	for _, p := range peers {
		if p.Id() == id {
			return p
		}
	}
	return nil
}

func OnTick(tick int, peers []*Peer, buffers map[PeerIdentity]chan BufferMessage, isSeed bool, mp *PieceMap) {
    for _, p := range peers {
        p.Stats().Update()
    }

    // Run choking algorithm
    if tick%chokeInterval == 0 {
        old, new, chokes, unchokes := ChokePeers(isSeed, peers, tick%optimisticChokeInterval == 0)
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
            buffers[p.Id()] <- OnSendMessages([]ProtocolMessage{ Choke{} }, p, mp)
        }
        // Send unchokes
        for _, p := range unchokes {
            buffers[p.Id()] <- OnSendMessages([]ProtocolMessage{ Unchoke{} }, p, mp)
        }
    }

    // Run piece picking algorithm
    pp := PickPieces(peers, mp)
    for p, blocks := range pp {
        buffers[p.Id()] <- OnSendMessages(blocks, p, mp)
    }
}