package bittorrent

type PeerRequestQueue struct {
	new []*RequestMessage
	pending []*RequestMessage
	received  []*BlockMessage
	max int
}

func NewPeerRequestQueue(max int) *PeerRequestQueue {
	return &PeerRequestQueue {
		new: make([]*RequestMessage, 0, max),
		pending: make([]*RequestMessage, 0, max),
		max : max,
	}
}

func (pq * PeerRequestQueue) Clear() {
	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.new = pq.new[:0]
	pq.pending = pq.pending[:0]
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
	return len(pq.new) + len(pq.pending) < pq.max
}

func (pq * PeerRequestQueue) AddRequest(index, begin, length uint32) {
	pq.new = append(pq.new, Request(index, begin, length))
}

func (pq * PeerRequestQueue) AddBlock(index, begin uint32, block []byte) {
	// If not in "pending" queue - discard...
}

func (pq * PeerRequestQueue) PumpQueue(in *RequestMessage, out chan *BlockMessage) {

	// REMOTE PEER => in (disk), out (outgoing buffer)
	// LOCAL PEER  => in (outgoing buffer), out(disk)

	// Pump requests to "in"

	// Pump blocks to "out"
}
