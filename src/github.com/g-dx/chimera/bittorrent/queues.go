package bittorrent

type PeerRequestQueue struct {
	newRequests     []*RequestMessage
	pendingRequests []*RequestMessage
	receivedBlocks  []*BlockMessage
	maxRequests int
}

func NewPeerRequestQueue(maxRequests int) *PeerRequestQueue {
	return &PeerRequestQueue {
		newRequests: make([]*RequestMessage, 0, maxRequests),
		pendingRequests: make([]*RequestMessage, 0, maxRequests),
		maxRequests : maxRequests,
	}
}

func (pq * PeerRequestQueue) Clear() {
	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.newRequests = pq.newRequests[:0]
	pq.pendingRequests = pq.pendingRequests[:0]
}

func (pq * PeerRequestQueue) Remove(index, begin, length uint32) bool {
	for i, req := range pq.newRequests {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			pq.newRequests = append(pq.newRequests[:i], pq.newRequests[i+1:]...)
			return true
		}
	}

	for i, req := range pq.pendingRequests {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			pq.pendingRequests = append(pq.pendingRequests[:i], pq.pendingRequests[i+1:]...)
			return true
		}
	}

	return false
}

func (pq * PeerRequestQueue) IsFull() bool {
	return len(pq.newRequests) + len(pq.pendingRequests) < pq.maxRequests
}

func (pq * PeerRequestQueue) AddRequest(index, begin, length uint32) {
	pq.newRequests = append(pq.newRequests, Request(index, begin, length))
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
