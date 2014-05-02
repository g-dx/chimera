package bittorrent

// IO Utils - ...

type OutgoingBuffer struct {
	buffer []ProtocolMessage
	c chan<- ProtocolMessage
}

func NewOutgoingBuffer(c chan<- ProtocolMessage, cap int) *OutgoingBuffer {
	return &OutgoingBuffer {
		buffer : make([]ProtocolMessage, 0, cap),
		c : c,
	}
}

func (ob * OutgoingBuffer) Size() int {
	return len(ob.buffer)
}

func (ob * OutgoingBuffer) Add(msg ProtocolMessage) {
	ob.buffer = append(ob.buffer, msg)
}

func (ob * OutgoingBuffer) Pump(max int) int {
	n := 0
	for i := 0 ; len(ob.buffer) > 0 && i < max ; i++ {
		select {
		case ob.c <- ob.buffer[0]:
			ob.buffer = ob.buffer[1:]
			n++
		default: break // Non-blocking...
		}
	}
	return n
}

func MaybeEnable(ch chan interface {}, f func() bool) chan interface {} {
	var channel chan interface {} // Nil
	if(f()) {
		channel = ch
	}
	return channel
}

type PeerRequestQueue struct {
	new      []*RequestMessage
	pending  []*RequestMessage
	received []*BlockMessage
	cap int
}

func NewPeerRequestQueue(cap int) *PeerRequestQueue {
	return &PeerRequestQueue {
		new: make([]*RequestMessage, 0, cap),
		pending: make([]*RequestMessage, 0, cap),
		cap : cap,
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
	return pq.Size() < pq.cap
}

func (pq * PeerRequestQueue) Size() int {
	return len(pq.new) + len(pq.pending)
}

func (pq * PeerRequestQueue) Capacity() int {
	return pq.cap
}

func (pq * PeerRequestQueue) AddRequest(r * RequestMessage) {
	pq.new = append(pq.new, r)
}

func (pq * PeerRequestQueue) AddBlock(b * BlockMessage) {
	// TODO: If not in pending queue - discard...
}

func (pq * PeerRequestQueue) Drain(requests, blocks * OutgoingBuffer) {

	// Drain requests out
	// TODO: Should we drain the entire queue?
	if len(pq.new) > 0 {
		requests.Add(pq.new[0])
		pq.pending = append(pq.pending, pq.new[0])
		pq.new = pq.new[1:]
	}

	// Drain blocks out
	// TODO: Should we drain the entire queue?
	if len(pq.received) > 0 {
		blocks.Add(pq.received[0])
		pq.received = pq.received[1:]
	}
}
