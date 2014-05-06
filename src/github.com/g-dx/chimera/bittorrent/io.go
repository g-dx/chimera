package bittorrent

// IO Utils - ...

type RequestMessageSink interface {
	CanWrite() bool
	WriteRequest(r *RequestMessage)
}

type BlockMessageSink interface {
	CanWrite() bool
	WriteBlock(b *BlockMessage)
}

// ----------------------------------------------------------------------------------
// BufferSink - writes messages to a RingBuffer
// ----------------------------------------------------------------------------------

type BufferSink struct {
	buffer *RingBuffer
}

func NewBufferSink(buffer *RingBuffer) *BufferSink {
	return &BufferSink {
		buffer : buffer,
	}
}

func (bs BufferSink) CanWrite() bool {
	return !bs.buffer.IsFull()
}

func (bs BufferSink) WriteBlock(m *BlockMessage) {
	bs.buffer.Add(m)
}

func (bs BufferSink) WriteRequest(m *RequestMessage) {
	bs.buffer.Add(m)
}


// ----------------------------------------------------------------------------------
// DiskSink - writes messages to a channel of DiskMessages
// ----------------------------------------------------------------------------------

type DiskSink struct {
	id PeerIdentity
	buffer chan DiskMessage
	cap, size int
}

func NewDiskSink(id PeerIdentity) *DiskSink {
	return &DiskSink{
		id : id,
		buffer: make(chan DiskMessage, 10),
	}
}

func (ds DiskSink) CanWrite() bool {
	return ds.size < ds.cap
}

func (ds * DiskSink) WriteBlock(msg *BlockMessage) {

	//	ds.buffer = append(ds.buffer, dm)
	ds.size++
}

func (ds * DiskSink) WriteRequest(msg *RequestMessage) {
	ds.size++
}

func (ds DiskSink) Flush(disk chan<- DiskMessage) {
	for ds.size > 0 {
		msg := <- ds.buffer
		select {
		case disk <- msg: ds.size--
		default: break // Non-blocking
		}
	}
}

// ----------------------------------------------------------------------------------
// PeerRequestQueue - manages requests for blocks
// ----------------------------------------------------------------------------------

type PeerRequestQueue struct {
	new      []*RequestMessage
	pending  []*RequestMessage
	received []*BlockMessage
	cap      int
}

func NewPeerRequestQueue(cap int) *PeerRequestQueue {
	return &PeerRequestQueue {
		new     : make([]*RequestMessage, 0, cap),
		pending : make([]*RequestMessage, 0, cap),
		cap     : cap,
	}
}

func (pq * PeerRequestQueue) ClearNew() []*RequestMessage {

	// Collect all outstanding new requests
	var reqs []*RequestMessage
	reqs = append(reqs, pq.new...)

	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.new = pq.new[:0]
	return reqs
}

func (pq * PeerRequestQueue) ClearAll() []*RequestMessage {

	// Collect all outstanding requests
	var reqs []*RequestMessage
	reqs = append(reqs, pq.ClearNew()...)
	reqs = append(reqs, pq.pending...)

	// NOTE: This does not make the elements of the slice eligible for GC!
	pq.pending = pq.pending[:0]
	return reqs
}

func (pq * PeerRequestQueue) Remove(index, begin, length uint32) bool {

	ok, reqs := removeRequest(index, begin, length, pq.new)
	if ok {
		pq.new = reqs
		return ok
	}

	ok, reqs = removeRequest(index, begin, length, pq.pending)
	if ok {
		pq.pending = reqs
		return ok
	}
	return false
}

func removeRequest(index, begin, length uint32, reqs []*RequestMessage) (bool, []*RequestMessage) {
	for i, req := range reqs {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			return true, append(reqs[:i], reqs[i+1:]...)
		}
	}
	return false, nil
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

func (pq * PeerRequestQueue) Drain(rSink RequestMessageSink, bSink BlockMessageSink) {

	for {
		ok := false
		if len(pq.new) > 0 && rSink.CanWrite() {
			rSink.WriteRequest(pq.new[0])
			pq.pending = append(pq.pending, pq.new[0])
			pq.new = pq.new[1:]
			ok = true
		}

		if len(pq.received) > 0 && bSink.CanWrite() {
			bSink.WriteBlock(pq.received[0])
			pq.received = pq.received[1:]
			ok = true
		}

		if !ok {
			break
		}
	}
}
