package bittorrent

import (
	"fmt"
	"log"
	"time"
	"io"
)

const (
	chokeInterval           = 10
	optimisticChokeInterval = 30
	idealPeers              = 25
	oneSecond 				= 1 * time.Second
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
	c *PeerConnection
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

func protocolLoop(c ProtocolConfig, mp *PieceMap, io ProtocolIO, logger *log.Logger) {

	buffers := make(map[PeerIdentity]chan BufferMessage)
	peers := make([]*Peer, 0, idealPeers)
	isSeed := mp.IsComplete()
	tick := 0

    // Define function for socket send
    toSocket := func(p *Peer, msgs []ProtocolMessage) {
        buffers[p.Id()] <- OnSendMessages(msgs, p, mp) // TODO: Externalise
    }

    // Define function for disk send
    toDisk := func(p *Peer, msgs []DiskMessage) {
        for _, msg := range msgs {
            io.dIn <- msg
        }
    }

    // Define function for socket broadcast
    toAllSockets := func(pm ProtocolMessage) {
        for id, buffer := range buffers {
            _, p := findPeer(id, peers)
            buffer <- OnSendMessages([]ProtocolMessage{ pm }, p, mp) // TODO: Externalise
        }
    }

	for {

		select {
		case <-io.tick:
			onTick(tick, peers, toSocket, isSeed, mp)
			tick++

		case r := <-io.trOut:
			addrs := onTrackerResponse(r, len(peers))
			for _, pa := range addrs {
				go handlePeerConnect(pa, c, logger, io)
			}

		case list := <-io.pMsgs:
			if ok, p := findPeer(list.id, peers); ok {
                err := onProtocolMessages(p, list.msgs, toSocket, toDisk, mp)
                if err != nil {
                    closePeer(p, err)
                }
            }

		case e := <-io.pErrs:
            if ok, p := findPeer(e.id, peers); ok {
                closePeer(p, e.err)
            }

		case wrapper := <-io.pNew:

			// TODO: The connection has been passed here and we should hang on to it!

			// Create log file & loggers
			err, file := c.w(fmt.Sprintf("%v.log", wrapper.c.in.conn.RemoteAddr()))
			if err != nil {
				logger.Fatalf("Can't create log file: [%v]\n", err)
			}

			// Create outgoing buffer & start connection
			in, out := Buffer(c.peerOutgoingBufferSize)
			wrapper.c.Start(wrapper.p.Id(), out, io.pMsgs, io.pErrs, file)

			// Store
			peers = append(peers, wrapper.p)
			buffers[wrapper.p.Id()] = in

		case m := <-io.dOut:
            switch msg := m.(type) {
                case ReadOk:
                    if ok, p := findPeer(msg.id, peers); ok {
                        onReadOk(msg.block, p, toSocket)
                    }
                case HashFailed:
                    onHashFailed(int(msg), mp)
                case PieceOk:
                    onPieceOk(int(msg), mp, toAllSockets, io.complete, logger)
                case ErrorResult:
                    onErrorResult(logger, msg.op, msg.err)
                case WriteOk: // Nothing to do...
                default:
                    logger.Printf("Unknown Disk Message: %v", msg)
            }

		case conn := <-io.pConns:
			if len(peers) < idealPeers {
				go handlePeerEstablish(conn, c, logger, io, false)
			}
		}
	}
}

func onReadOk(b Block, p *Peer, toSocket func(*Peer, []ProtocolMessage)) {
    toSocket(p, []ProtocolMessage{ b })
}

func onHashFailed(index int, mp *PieceMap) {
    mp.Piece(index).Reset()
}

func onErrorResult(l *log.Logger, op string, err error) {
    l.Panicf("Disk [%v] Failed: %v\n", op, err)
}

func onPieceOk(index int, mp *PieceMap, toAllSockets func(ProtocolMessage), complete chan struct{}, l *log.Logger) {

    l.Printf("piece [%v] written to disk\n", index)
    mp.Get(index).Complete()
    toAllSockets(Have(index))

    if mp.IsComplete() {
        close(complete) // Complete! Yay!
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

	// Attempt to establish connection
	id, err := conn.Establish(Handshake(c.mi.InfoHash), outgoing)
	if err != nil {
		logger.Printf("Can't establish connection [%v]: %v\n", conn.in.conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// Connected
	logger.Printf("New Peer: %v\n", id)
	io.pNew <- PeerWrapper{NewPeer(id, len(c.mi.Hashes)), conn}
}

func closePeer(peer *Peer, err error) {

	// TODO:

	// 1. Remove from peers
	// 2. peer.Close()
	// 3. Log errors
}

func findPeer(id PeerIdentity, peers []*Peer) (bool, *Peer) {
	for _, p := range peers {
		if p.Id() == id {
			return true, p
		}
	}
	return false, nil
}

func onTick(tick int, peers []*Peer, send func(*Peer, []ProtocolMessage), isSeed bool, mp *PieceMap) {

    // Update stats
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
            send(p, []ProtocolMessage{ Choke{} })
        }
        // Send unchokes
        for _, p := range unchokes {
            send(p, []ProtocolMessage{ Unchoke{} })
        }
    }

    // Run piece picking algorithm
    pp := PickPieces(peers, mp)
    for p, blocks := range pp {
        send(p, blocks)
    }
}

func onProtocolMessages(p *Peer, msgs []ProtocolMessage,
                        toSocket func(*Peer, []ProtocolMessage),
                        toDisk func(*Peer, []DiskMessage), mp *PieceMap) error {
    // Process all messages
    err, net, disk := OnReceiveMessages(msgs, p, mp)
    if err != nil {
        return err
    }
    // Send
    toSocket(p, net)
    toDisk(p, disk)
    return nil
}