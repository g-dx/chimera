package bittorrent

import (
	"time"
	"fmt"
	"log"
	"os"
)

var (
	QUARTER_OF_A_SECOND = 250 * time.Millisecond
	FIFTY_MILLISECONDS = 50 * time.Millisecond
	idealPeers = 5
)

type PeerCoordinator struct {
	metaInfo *MetaInfo
	peers []Peer
	trackerResponses <-chan *TrackerResponse
	addPeer chan Peer
	pieceMap *PieceMap
	done chan struct{}
	dir string
	logger *log.Logger
}

func NewPeerCoordinator(mi *MetaInfo, dir string, tr <-chan *TrackerResponse) (*PeerCoordinator, error) {

	// Create log file
	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/coordinator.log", dir))
	if err != nil {
		return nil, err
	}

	// Create piece map
	pieceMap := NewPieceMap(len(mi.Hashes))

	// Create coordinator
	pc := &PeerCoordinator{
		metaInfo : mi,
		peers : make([]Peer, 0, idealPeers),
		trackerResponses : tr,
		addPeer : make(chan Peer),
		pieceMap : pieceMap,
		done : make(chan struct{}),
		dir : dir,
		logger : log.New(f, "", log.Ldate | log.Ltime),
	}

	// Start loop & return
	go pc.loop()
	return pc, nil
}

func (pc * PeerCoordinator) AwaitDone() {
	// Await a receive to say we are finished...
	<- pc.done
}

func (pc * PeerCoordinator) loop() {

	for {

		select {

		case <- time.After(1 * time.Second):
			// Run downloader...

		case <- time.After(10 * time.Second):
			// Run choking algorithm

		case r := <- pc.trackerResponses:
			pc.onTrackerResponse(r)

		case p := <- pc.addPeer:
			pc.peers = append(pc.peers, p)

		default:
			pc.processMessagesFor(QUARTER_OF_A_SECOND)
		}
	}
}

func (pc * PeerCoordinator) processMessagesFor(d time.Duration) {

	pc.logger.Println("[Coordinator] - processing messages")

	// Set a maximum amount of
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {

		msgs := 0
		for _, p := range pc.peers {
			msgs += p.ProcessMessages(nil, nil)
		}

		// If we did nothing this loop - wait for more data to arrive
		if msgs == 0 {
			time.Sleep(FIFTY_MILLISECONDS)
		}
	}
}

func (pc * PeerCoordinator) onTrackerResponse(r *TrackerResponse) {

	peerCount := len(pc.peers)
	if peerCount < idealPeers {

		// Add some
		for _, pa := range r.PeerAddresses[25:] {
			go pc.handlePeerConnect(pa)
			peerCount++
			if peerCount == idealPeers {
				break
			}
		}
	}
}

func (pc * PeerCoordinator) handlePeerConnect(addr PeerAddress) {

	c, err := NewConnection(addr.GetIpAndPort())
	if err != nil {
		fmt.Println("Error: ", err)
		// TODO: Should log this as can't connect or something...
		return
	}

	in := make(chan ProtocolMessage)
	out := make(chan ProtocolMessage)
	e := make(chan error)
	outHandshake := Handshake(pc.metaInfo.InfoHash)

	// Attempt to establish connection
	id, err := c.Establish(in, out, e, outHandshake, pc.dir)
	if err != nil {
		fmt.Println("Error: ", err)
		c.Close()
		return;
	}

	// Connected
	p := NewPeer(*id, in, out, pc.logger)
	fmt.Printf("New Peer: %v\n", p)
	pc.addPeer <- p
}
