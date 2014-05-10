package bittorrent

import "time"

// ----------------------------------------------------------------------------------
// MessageSink - manages destination for requests, blocks, protocol traffic...
// ----------------------------------------------------------------------------------

type MessageSink struct {
	id PeerIdentity
	conn chan<- ProtocolMessage
	disk chan<- DiskMessage
	w func(ProtocolMessage) bool // Decides which channel to write to
}

func (ms MessageSink) Write(msg ProtocolMessage) bool {
	return ms.w(msg)
}


func (ms MessageSink) ToConnection(msg ProtocolMessage) bool {
	select {
	case ms.conn <- msg : return true
	default: return false
	}
}

func (ms MessageSink) ToDisk(msg DiskMessage) bool {
	select {
	case ms.disk <- msg : return true
	default: return false
	}
}

func NewRemoteMessageSink(id PeerIdentity,
	                      conn chan<- ProtocolMessage,
						  disk chan<- DiskMessage) *MessageSink {

	ms := &MessageSink { id : id, conn : conn, disk : disk, }
	ms.w = func (m ProtocolMessage) bool {
		switch msg := m.(type) {
		case *RequestMessage: return ms.ToDisk(DiskRead(msg, id))
		default: return ms.ToConnection(msg)
		}
	}
	return ms
}

func NewLocalMessageSink(id PeerIdentity,
						 conn chan<- ProtocolMessage,
						 disk chan<- DiskMessage) *MessageSink {
	ms := &MessageSink { id : id, conn : conn, disk : disk, }
	ms.w = func (m ProtocolMessage) bool {
		switch msg := m.(type) {
		case *BlockMessage: return ms.ToDisk(DiskWrite(msg, id))
		default: return ms.ToConnection(msg)
		}
	}
	return ms
}

// ----------------------------------------------------------------------------------
// PeerRequestQueue - manages requests for blocks
// ----------------------------------------------------------------------------------

type PeerRequestQueue struct {
	new      []*TimedRequestMessage  // Newly received (from network or block picker) requests
	pending  []*TimedRequestMessage  // Submitted (to disk or network) requests
	received []*BlockMessage    // Received blocks (from network or disk)
	other    [] ProtocolMessage // All other protocol messages (Have, Interested, etc)
	cap      int
}

type TimedRequestMessage struct {
	req *RequestMessage
	start time.Time
}

func NewPeerRequestQueue(cap int) *PeerRequestQueue {
	return &PeerRequestQueue {
		new      : make([]*TimedRequestMessage, 0, cap),
		pending  : make([]*TimedRequestMessage, 0, cap),
		received : make([]*BlockMessage, 0, cap),
		other    : make([] ProtocolMessage, 0, cap),
		cap      : cap,
	}
}

func (pq * PeerRequestQueue) ClearNew() []*RequestMessage {

	// Collect all outstanding new requests
	reqs := make([]*RequestMessage, 0, len(pq.new))
	for _, req := range pq.new {
		reqs = append(reqs, req.req)
	}

	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.new = pq.new[:0]
	return reqs
}

func (pq * PeerRequestQueue) ClearExpired(t time.Duration) []*RequestMessage {

	// NOTE: Can optimise this by starting at the other end & breaking on the
	//       first value to not be expired.
	var reqs []*RequestMessage
	for i, req := range pq.new {
		if req.start.Add(t).After(time.Now()) {
			reqs = append(reqs, req.req)
			pq.new = append(pq.new[:i], pq.new[i+1:]...)
		}
	}

	for i, req := range pq.pending {
		if req.start.Add(t).After(time.Now()) {
			reqs = append(reqs, req.req)
			pq.pending = append(pq.pending[:i], pq.pending[i+1:]...)
		}
	}

	return reqs
}

func (pq * PeerRequestQueue) ClearAll() []*RequestMessage {

	// Collect all outstanding requests
	reqs := make([]*RequestMessage, 0, len(pq.new) + len(pq.pending))
	reqs = append(reqs, pq.ClearNew()...)
	for _, req := range pq.pending {
		reqs = append(reqs, req.req)
	}

	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.pending = pq.pending[:0]
	return reqs
}

func (pq * PeerRequestQueue) Remove(index, begin, length uint32) {

	var req *TimedRequestMessage
	req, pq.new = RemoveRequest(index, begin, length, pq.new)
	if req != nil {
		return
	}

	req, pq.pending = RemoveRequest(index, begin, length, pq.pending)
	if req != nil {
		return
	}
}

func RemoveRequest(index, begin, length uint32, reqs []*TimedRequestMessage) (*TimedRequestMessage, []*TimedRequestMessage) {
	for i, req := range reqs {
		if req.req.Index() == index &&
		   req.req.Begin() == begin &&
		   req.req.Length() == length {
			return reqs[i], append(reqs[:i], reqs[i+1:]...)
		}
	}
	return nil, reqs
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

func (pq * PeerRequestQueue) Add(m ProtocolMessage) {
	switch msg := m.(type) {
	case *BlockMessage:

		// If not in pending queue - skip block
		for i, req := range pq.pending {
			if req.req.Index() == msg.Index() &&
			   req.req.Begin() == msg.Begin() &&
			   req.req.Length() == uint32(len(msg.Block())) {
				pq.received = append(pq.received, msg)
				pq.pending = append(pq.pending[:i], pq.pending[i+1:]...)
				break
			}
		}

		// NOTE: Could add logic to check if we ever requested this

	case *RequestMessage: pq.new = append(pq.new, &TimedRequestMessage{msg, time.Now()})
	default: pq.other = append(pq.other, msg)
	}
}

func (pq * PeerRequestQueue) Drain(sink *MessageSink) int{

	// Attempt to drain all request & blocks
	n := 0
	for {
		req := false
		if len(pq.new) > 0 {
			ok := sink.Write(pq.new[0].req)
			if ok {
				pq.pending = append(pq.pending, pq.new[0])
				pq.new = pq.new[1:]
				req = true
				n++
			}
		}

		block := false
		if len(pq.received) > 0 {
			ok := sink.Write(pq.received[0])
			if ok {
				pq.received = pq.received[1:]
				block = true
				n++
			}
		}

		other := false
		if len(pq.other) > 0 {
			ok := sink.Write(pq.other[0])
			if ok {
				pq.other = pq.other[1:]
				other = true
				n++
			}
		}

		// If no I/O this loop break...
		if !req || !block || !other {
			break
		}
	}
	return n
}
