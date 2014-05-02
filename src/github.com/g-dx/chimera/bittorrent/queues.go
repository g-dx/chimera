package bittorrent

type PeerRequestQueue struct {
	new      []*RequestMessage
	pending  []*RequestMessage
	received []*BlockMessage
	max int
}

func NewPeerRequestQueue(max int) *PeerRequestQueue {
	return &PeerRequestQueue {
		new: make([]*RequestMessage, 0, max),
		pending: make([]*RequestMessage, 0, max),
		max : max,
	}
}

func (pq * PeerRequestQueue) Clear() []*RequestMessage {

	// Collect all outstanding requests
	var reqs []*RequestMessage
	reqs = append(reqs, pq.new...)
	reqs = append(reqs, pq.pending...)

	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.new = pq.new[:0]
	pq.pending = pq.pending[:0]
	return reqs
}

func (pq * PeerRequestQueue) Remove(index, begin, length uint32) bool {
	for i, req := range pq.new {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			pq.new = append(pq.new[:i], pq.new[i+1:]...)
			return true
		}
	}

	for i, req := range pq.pending {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			pq.pending = append(pq.pending[:i], pq.pending[i+1:]...)
			return true
		}
	}

	return false
}

func (pq * PeerRequestQueue) IsFull() bool {
	return pq.Size() < pq.max
}

func (pq * PeerRequestQueue) Size() int {
	return len(pq.new) + len(pq.pending)
}

func (pq * PeerRequestQueue) Capacity() int {
	return pq.max
}

func (pq * PeerRequestQueue) AddRequest(r * RequestMessage) {
	pq.new = append(pq.new, r)
}

func (pq * PeerRequestQueue) AddBlock(b * BlockMessage) {
	// TODO: If not in pending queue - discard...
}

func (pq * PeerRequestQueue) Pump(reqs, blocks chan<- ProtocolMessage,) {

	// REMOTE PEER => in (disk), out (outgoing buffer)
	// LOCAL PEER  => in (outgoing buffer), out(disk)

	// Pump requests to "in"

	// Pump blocks to "out"
}


//func (p * Peer) MaybeReadBlock(diskReader chan RequestMessage) {
//	if len(p.remotePendingReads) > 0 {
//		select {
//		case diskReader <- *p.remotePendingReads[0]:
//			p.remoteSubmittedReads = append(p.remoteSubmittedReads, p.remotePendingReads[0])
//			p.remotePendingReads = p.remotePendingReads[1:]
//		default:
//			// Non-blocking read...
//		}
//	}
//}
//
//func (p * Peer) MaybeWriteBlock(diskWriter chan BlockMessage) {
//	if len(p.localPendingWrites) > 0 {
//		select {
//		case diskWriter <- *p.localPendingWrites[0]:
//			p.localPendingWrites = p.localPendingWrites[1:]
//		default:
//			// Non-blocking write...
//		}
//	}
//}
