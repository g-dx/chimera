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
	idealPeers = 25
)

type PeerCoordinator struct {
	metaInfo *MetaInfo
	peers []*Peer
	trackerResponses <-chan *TrackerResponse
	addPeer chan *Peer
	pieceMap *PieceMap
	done chan struct{}
	dir string
	logger *log.Logger
	diskR chan<- ProtocolMessage
}

func NewPeerCoordinator(mi *MetaInfo, dir string, tr <-chan *TrackerResponse) (*PeerCoordinator, error) {

	// Create log file
	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/coordinator.log", dir))
	if err != nil {
		return nil, err
	}
	logger := log.New(f, "", log.Ldate | log.Ltime)

	// Start fake disk reader
	diskR := make(chan<- ProtocolMessage)
	go mockDisk(diskR, logger)


	// Create piece map
	pieceMap := NewPieceMap(uint32(len(mi.Hashes)), mi.PieceLength, mi.TotalLength())

	// Create coordinator
	pc := &PeerCoordinator{
		metaInfo : mi,
		peers : make([]*Peer, 0, idealPeers),
		trackerResponses : tr,
		addPeer : make(chan *Peer),
		pieceMap : pieceMap,
		done : make(chan struct{}),
		dir : dir,
		logger : logger,
		diskR : diskR,
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

	onPicker := time.After(1 * time.Second)
	for {

		select {

		case <- onPicker:
			PickPieces(pc.peers, pc.pieceMap)
			onPicker = time.After(1 * time.Second)

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

	// Set a maximum amount of
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {

		msgs := 0
		for _, p := range pc.peers {
			msgs += p.ProcessMessages()
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
			go pc.handlePeerConnect(pa, pc.pieceMap)
			peerCount++
			if peerCount == idealPeers {
				break
			}
		}
	}
}

func (pc * PeerCoordinator) handlePeerConnect(addr PeerAddress, pieceMap *PieceMap) {

	conn, err := NewConnection(addr.GetIpAndPort())
	if err != nil {
		pc.logger.Printf("Can't connect to [%v]: %v\n", addr, err)
		return
	}

	in := make(<-chan ProtocolMessage, 10)
	out := make(chan<- ProtocolMessage, 10)
	e := make(chan error, 3) // error sources -> reader, writer, peer
	outHandshake := Handshake(pc.metaInfo.InfoHash)

	// Attempt to establish connection
	id, err := conn.Establish(in, out, e, outHandshake, pc.dir)
	if err != nil {
		pc.logger.Printf("Can't establish connection [%v]: %v\n", addr, err)
		conn.Close()
		return;
	}

	// NOTE: Pass this function to peer and when it closes itself or
	//       close is called externally it calls back to cleanup coordinator
	onPeerClose := func(err error) {

		// 1. Log initial error which caused close (possibly nil)

		// 2. Close connection
		err = conn.Close();
		if err != nil {
			// Can't really do anything about it...
		}

		// 3. Remove from list of peers
//		pc.peers.remove();
	}

	// Connected
	p := NewPeer(*id, out, pc.diskR, in, pc.metaInfo, pieceMap, e, pc.logger, onPeerClose)
	pc.logger.Printf("New Peer: %v\n", p)
	pc.addPeer <- p
}
