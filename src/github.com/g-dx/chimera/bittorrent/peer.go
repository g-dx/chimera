package bittorrent

import (
	"fmt"
	"log"
)

var (
	maxOutgoingQueue = 25
	maxRemoteRequestQueue = 25
	maxOutstandingLocalRequests = 25
)

type ProtocolHandler interface {
	Choke()
	Unchoke()
	Interested()
	Uninterested()
	Have(index uint32)
	Bitfield(b []byte)
	Cancel(index, begin, length uint32)
	Request(index, begin, length uint32)
	Block(index, begin uint32, block []byte)
}

type Statistics struct {
	totalBytesDownloaded uint64
	bytesDownloadedPerUpdate uint
	bytesDownloaded uint
}

func (s * Statistics) Update() {

	// Update & reset
	s.bytesDownloadedPerUpdate = s.bytesDownloaded
	s.bytesDownloaded = 0
}

func (s * Statistics) Downloaded(n uint) {
	s.bytesDownloaded += n
}

type Peer struct {

	// General protocol traffic
	outBuf	   []ProtocolMessage

	// Remote requests received & requests submitted to disk reader
	remotePendingReads []*RequestMessage
	remoteSubmittedReads []*RequestMessage

	// Local reads sent & received pieces
	localPendingReads []*RequestMessage
	localPendingWrites []*BlockMessage

	out 	    chan<- ProtocolMessage
	in 			<-chan ProtocolMessage
	pieceMap 	*PieceMap
	bitfield 	*BitSet
	state PeerState
	id PeerIdentity
	logger * log.Logger
	stats * Statistics
}

func NewPeer(id PeerIdentity,
		     out chan<- ProtocolMessage,
			 in <-chan ProtocolMessage,
			 mi * MetaInfo,
	         pieceMap *PieceMap,
			 logger * log.Logger) *Peer {
	return &Peer {
		outBuf : make([]ProtocolMessage, 0, 25),
		remotePendingReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		remoteSubmittedReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		localPendingReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		localPendingWrites : make([]*BlockMessage, 0, maxRemoteRequestQueue),
		out : out,
		in : in,
		pieceMap : pieceMap,
		bitfield : NewBitSet(uint32(len(mi.Hashes))),
		state : NewPeerState(),
		id : id,
		logger : logger,
		stats : new(Statistics),
	}
}

func (p * Peer) CanWrite() bool {
	return len(p.outBuf) > 0
}

func (p * Peer) CanRead() bool {
	// Can read if there is space in request queue
	// TODO: should we consider the length of the protocol queue?
	return len(p.remotePendingReads) < maxRemoteRequestQueue && len(p.outBuf) < maxOutgoingQueue
}

func (p * Peer) ReadNextMessage() (readMessage bool) {

	select {
	case msg := <- p.in:
		readMessage = true
		p.handleMessage(msg)
	default:
		// Non-blocking read...
	}
	return readMessage
}

func (p * Peer) WriteNextMessage() (wroteMessage bool) {

	select {
	case p.out <- p.outBuf[0]:
		p.outBuf = p.outBuf[1:]
		wroteMessage = true
	default: // Non-blocking read...
	}
	return wroteMessage
}

type PeerState struct {
	remoteChoke, localChoke, remoteInterest, localInterest bool
}

func NewPeerState() PeerState {
	return PeerState {
		remoteChoke : true,
		localChoke : true,
		remoteInterest : false,
		localInterest : false,
	}
}

func (p * Peer) handleMessage(pm ProtocolMessage) {
	p.logger.Printf("%v, Handling Msg: %v\n", p.id, pm)
	switch msg := pm.(type) {
	case *ChokeMessage: p.Choke()
	case *UnchokeMessage: p.Unchoke()
	case *InterestedMessage: p.Interested()
	case *UninterestedMessage: p.Uninterested()
	case *BitfieldMessage: p.Bitfield(msg.Bits())
	case *HaveMessage: p.Have(msg.Index())
	case *CancelMessage: p.Cancel(msg.Index(), msg.Begin(), msg.Length())
	case *RequestMessage: p.Request(msg.Index(), msg.Begin(), msg.Length())
	case *BlockMessage: p.Block(msg.Index(), msg.Begin(), msg.Block())
	default:
		panic(fmt.Sprintf("Unknown protocol message: %v", pm))
	}
}

func (p * Peer) Choke() {
	// Choke:
	// 1. Return blocks
	// 2. Clear local request queue
	// 3. Set state
	p.pieceMap.ReturnBlocks(p.localPendingReads)
	p.localPendingReads = make([]*RequestMessage, 0, maxOutstandingLocalRequests)
	p.state.localChoke = true
}

func (p * Peer) Unchoke() {
	// Unchoke:
	// 1. set state
	// 2. TODO: Should have requests ready to write
	p.state.localChoke = false
}

func (p * Peer) Interested() {
	// Interested
	// 1. Set state, ensure they aren't a peer
	p.state.remoteInterest = !p.bitfield.IsComplete()
}

func (p * Peer) Uninterested() {
	// Uninterested
	// 1. Set state, ensure they aren't a peer
	p.state.remoteInterest = false
}

func (p * Peer) Have(index uint32) {
	// Have
	// 1. ensure valid index & not already in possession
	// 2. check if we need piece - if so send interest
	if !p.bitfield.IsValid(index) {
		die(fmt.Sprintf("Invalid index received: %v", index))
	}

	if !p.bitfield.Have(index) {
		if p.isNowInteresting(index) {
			p.send(Interested)
		}
		p.bitfield.Set(index)
		p.state.remoteInterest = !p.bitfield.IsComplete()
		p.pieceMap.Inc(index)
	}
}

func (p * Peer) Cancel(index, begin, length uint32) {
	// Cancel:
	// 1. Remove all remote peer requests for this piece
	for i, req := range p.remotePendingReads {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			p.remotePendingReads = append(p.remotePendingReads[:i], p.remotePendingReads[i+1:]...)
		}
	}

	for i, req := range p.remoteSubmittedReads {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			p.remoteSubmittedReads = append(p.remoteSubmittedReads[:i], p.remoteSubmittedReads[i+1:]...)
		}
	}
}

func (p * Peer) Request(index, begin, length uint32) {
	// Check index, begin & length is valid - if not close connection
	if !p.pieceMap.IsValid(index, begin, length) {
		die(fmt.Sprintf("Invalid request received: %v, %v, %v", index, begin, length))
	}

	if !p.state.remoteChoke {
		// Add request to pending reads queue
		p.remotePendingReads = append(p.remotePendingReads, Request(index, begin, length))
	}
}

func (p * Peer) Block(index, begin uint32, block []byte) {
	// Check index, begin & length is valid and we requested this piece
	if !p.pieceMap.IsValid(index, begin, uint32(len(block))) {
		die(fmt.Sprintf("Invalid piece received: %v, %v, %v", index, begin, block))
	}

	// Remove from pending write queue
	// TODO: Move this into its own function; req, err := p.removeOutgoingRequest(index, begin)
	for i, req := range p.localPendingReads {
		if req.Index() == index && req.Begin() == begin {
			p.localPendingReads = append(p.localPendingReads[:i], p.localPendingReads[i+1:]...)
			break
		}
	}

	// To pending queue
	p.localPendingWrites = append(p.localPendingWrites, Block(index, begin, block))
	p.Stats().Downloaded(uint(len(block)))
}

func (p * Peer) Bitfield(b []byte) {
	// Check required high bits are set to zero
	// TODO: Validate bitfield

	// Create new bitfield
	p.bitfield = NewFromBytes(b, p.bitfield.Size())

	// Add to global piece map
	p.pieceMap.IncAll(p.bitfield)

	// Check if we are interested
	for i := uint32(0); i < p.bitfield.Size(); i++ {
		if p.pieceMap.Piece(i).BlocksNeeded() {
			p.state.localInterest = true
			break
		}
	}
}

func (p * Peer) ProcessMessages(diskReader chan RequestMessage, diskWriter chan BlockMessage) int {

	// Process reads
	io := false
	r := p.CanRead()
//	p.logger.Printf("%v, Can Read: %v\n", p.id, r)
	if r {
		io = io || p.ReadNextMessage()
	}

	// Process read & write requests (if any)
	p.readBlock(diskReader)
	p.writeBlock(diskWriter)
	io = io || len(p.remotePendingReads) > 0
	io = io || len(p.localPendingWrites) > 0

	// Process writes
	w := p.CanWrite()
//	p.logger.Printf("%v, Can Write: %v\n", p.id, w)
	if w {
		io = io || p.WriteNextMessage()
	}

	// Dear god....
	if io {
		return 1
	} else {
		return 0
	}
}

func (p Peer) Stats() *Statistics {
	return p.stats

}

func (p * Peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).BlocksNeeded()
}

func (p * Peer) send(msg ProtocolMessage) {
	p.outBuf = append(p.outBuf, msg)
}

func die(msg string) {
	fmt.Println(msg)
}

func (p * Peer) readBlock(diskReader chan RequestMessage) {
	if len(p.remotePendingReads) > 0 {
		select {
		case diskReader <- *p.remotePendingReads[0]:
			p.remoteSubmittedReads = append(p.remoteSubmittedReads, p.remotePendingReads[0])
			p.remotePendingReads = p.remotePendingReads[1:]
		default:
			// Non-blocking...
		}
	}
}

func (p * Peer) writeBlock(diskWriter chan BlockMessage) {
	if len(p.localPendingWrites) > 0 {
		select {
		case diskWriter <- *p.localPendingWrites[0]:
			p.localPendingWrites = p.localPendingWrites[1:]
		default:
			// Non-blocking write...
		}
	}
}

func (p * Peer) onBlockRead(index, begin uint32, block []byte) {

	// Attempt to remove from read requests
	removed := false
	for i, req := range p.remoteSubmittedReads {
		if req.Index() == index && req.Begin() == begin {
			p.remoteSubmittedReads = append(p.remoteSubmittedReads[:i], p.remoteSubmittedReads[i+1:]...)
			removed = true
		}
	}

	// if found add to outgoing queue
	if removed {
		p.send(Block(index, begin, block))
	}
}

func (p * Peer) AddOutgoingRequest(req *RequestMessage) {
	p.localPendingReads = append(p.localPendingReads, req)
	p.outBuf = append(p.outBuf, req)
}

func (p * Peer) BlocksRequired() uint {
	return uint(maxRemoteRequestQueue - len(p.localPendingReads))
}

func (p * Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest && p.BlocksRequired() > 0
}

func (p * Peer) String() string {
	return fmt.Sprintf("")
}
