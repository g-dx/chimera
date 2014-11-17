package bittorrent

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	FIFTY_MILLISECONDS = 50 * time.Millisecond
	idealPeers         = 25
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
	peerMsgs         chan *ProtocolMessageWithId
	peerErrors       chan PeerError
	heartbeat        int64
}

func NewProtocolHandler(mi *MetaInfo, dir string, tr <-chan *TrackerResponse) (*ProtocolHandler, error) {

	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/protocol.log", dir))
	if err != nil {
		return nil, err
	}
	logger := log.New(f, "", log.Ldate|log.Ltime)

	// Create piece map
	pieceMap := NewPieceMap(uint32(len(mi.Hashes)), mi.PieceLength, mi.TotalLength())

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
		peerMsgs:         make(chan *ProtocolMessageWithId, 100),
		peerErrors:       make(chan PeerError),
	}

	// Start loop & return
	go ph.loop()
	return ph, nil
}

func (ph *ProtocolHandler) AwaitDone() {
	// Await a receive to say we are finished...
	<-ph.done
}

func (ph *ProtocolHandler) loop() {

	for {

		select {
		case <-time.After(ONE_SECOND):
			ph.onHeartbeat()

		case r := <-ph.trackerResponses:
			ph.onTrackerResponse(r)

		case m := <-ph.peerMsgs:
			ph.onPeerMessage(m.PeerId(), m.Msg())

		case e := <-ph.peerErrors:
			ph.onPeerError(e.Id(), e.Error())

		case p := <-ph.newPeers:
			ph.peers = append(ph.peers, p)
		}
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

	in := make(chan ProtocolMessage, 10)
	outHandshake := Handshake(ph.metaInfo.InfoHash)

	// Attempt to establish connection
	id, err := conn.Establish(in, ph.peerMsgs, ph.peerErrors, outHandshake, ph.dir)
	if err != nil {
		ph.logger.Printf("Can't establish connection [%v]: %v\n", addr, err)
		conn.Close()
		return
	}

	// Connected
	p := NewPeer(id, in, ph.metaInfo, ph.pieceMap, ph.logger)
	ph.logger.Printf("New Peer: %v\n", p)
	ph.newPeers <- p
}

func (ph *ProtocolHandler) onPeerMessage(id *PeerIdentity, msg ProtocolMessage) {

	p := ph.findPeer(id)
	if p != nil {
		err := p.OnMessage(msg)
		if err != nil {
			ph.closePeer(p, err)
		}
	}

	// TODO: msg.Dispose() -> Pool.Put(msg)

}

func (ph *ProtocolHandler) closePeer(peer *Peer, err error) {

	// TODO:

	// 1. Remove from peers
	// 2. peer.Close()
	// 3. Log errors
}

func (ph *ProtocolHandler) onHeartbeat() {

	// Run choking algorithm
	if ph.heartbeat%10 == 0 {
		// Run choker
	}

	// Run piece picking algorithm
	PickPieces(ph.peers, ph.pieceMap)

	// Inc heartbeat
	ph.heartbeat++
}

func (ph *ProtocolHandler) onPeerError(id *PeerIdentity, err error) {

	p := ph.findPeer(id)
	if p != nil {
		ph.closePeer(p, err)
	}
}

func (ph *ProtocolHandler) maybeConnect(r chan<- PeerConnectResult) {

	peerCount := len(ph.peers)
	if peerCount < idealPeers {

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
