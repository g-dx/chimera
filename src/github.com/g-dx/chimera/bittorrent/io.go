package bittorrent

import "time"

// ----------------------------------------------------------------------------------
// MessageSink - manages destination for requests, blocks, protocol traffic...
// ----------------------------------------------------------------------------------

type MessageSink struct {
	id   *PeerIdentity
	conn chan<- ProtocolMessage
	disk chan<- DiskMessage
	w    func(ProtocolMessage) bool // Decides which channel to write to
}

func (ms MessageSink) Write(msg ProtocolMessage) bool {
	return ms.w(msg)
}

func (ms MessageSink) ToConnection(msg ProtocolMessage) bool {
	select {
	case ms.conn <- msg:
		return true
	default:
		return false
	}
}

func (ms MessageSink) ToDisk(msg DiskMessage) bool {
	select {
	case ms.disk <- msg:
		return true
	default:
		return false
	}
}

func NewRemoteMessageSink(id *PeerIdentity,
	conn chan<- ProtocolMessage,
	disk chan<- DiskMessage) *MessageSink {

	ms := &MessageSink{id: id, conn: conn, disk: disk}
	ms.w = func(m ProtocolMessage) bool {
		switch msg := m.(type) {
		case *RequestMessage:
			return ms.ToDisk(DiskRead(msg, id))
		default:
			return ms.ToConnection(msg)
		}
	}
	return ms
}

func NewLocalMessageSink(id *PeerIdentity,
	conn chan<- ProtocolMessage,
	disk chan<- DiskMessage) *MessageSink {
	ms := &MessageSink{id: id, conn: conn, disk: disk}
	ms.w = func(m ProtocolMessage) bool {
		switch msg := m.(type) {
		case *BlockMessage:
			return ms.ToDisk(DiskWrite(msg, id))
		default:
			return ms.ToConnection(msg)
		}
	}
	return ms
}

// ----------------------------------------------------------------------------------
// BlockRequestStatus - tracks the progress of requesting blocks
// ----------------------------------------------------------------------------------

const (
	new = iota
	pending
	received
)

type BlockRequestStatus struct {
	state int
	req   *RequestMessage
	block *BlockMessage
	start time.Time
}

func NewBlockRequestStatus(req *RequestMessage) *BlockRequestStatus {
	return &BlockRequestStatus{
		state: new,
		req:   req,
		block: nil,
		start: time.Now(),
	}
}

// ----------------------------------------------------------------------------------
// MessageQueue - manages protocol messages
//
//  <- removed  | RECEIVED | RECEIVED | PENDING | PENDING |  NEW  |  <- added
//
// ----------------------------------------------------------------------------------

type MessageQueue struct {
	reqs  []*BlockRequestStatus
	other []ProtocolMessage
	cap   int
}

func NewMessageQueue(cap int) *MessageQueue {
	return &MessageQueue{
		reqs:  make([]*BlockRequestStatus, 0, cap),
		other: make([]ProtocolMessage, 0, cap),
		cap:   cap,
	}
}

func (mq *MessageQueue) ClearNew() []*RequestMessage {
	return mq.Clear(func(brt *BlockRequestStatus) bool {
		return brt.state == new
	})
}

func (mq *MessageQueue) ClearExpired(t time.Duration) []*RequestMessage {
	return mq.Clear(func(brt *BlockRequestStatus) bool {
		return (brt.state == new || brt.state == pending) && brt.start.Add(t).After(time.Now())
	})
}

func (mq *MessageQueue) ClearAll() []*RequestMessage {
	return mq.Clear(func(_ *BlockRequestStatus) bool { return true })
}

func (mq *MessageQueue) Remove(index, begin, length uint32) {
	mq.Clear(func(brt *BlockRequestStatus) bool {
		return brt.req.Index() == index &&
			brt.req.Begin() == begin &&
			brt.req.Length() == length
	})
}

func (mq *MessageQueue) Clear(p func(*BlockRequestStatus) bool) []*RequestMessage {

	// Collect all cleared requests
	reqs := make([]*RequestMessage, 0, mq.cap)
	for i, req := range mq.reqs {
		if p(req) {
			// Remove & append to cleared list
			mq.reqs = append(mq.reqs[:i], mq.reqs[i+1:]...)
			reqs = append(reqs, req.req)
		}
	}
	return reqs
}

func (mq *MessageQueue) IsFull() bool {
	return mq.Size() < mq.cap
}

func (mq *MessageQueue) Size() int {
	return len(mq.reqs)
}

func (mq *MessageQueue) Capacity() int {
	return mq.cap
}

func (mq *MessageQueue) Add(m ProtocolMessage) {
	switch msg := m.(type) {
	case *BlockMessage:

		// Check we have pending request for this block, otherwise skip
		for i, req := range mq.reqs {
			if req.req.Index() == msg.Index() &&
				req.req.Begin() == msg.Begin() &&
				req.req.Length() == uint32(len(msg.Block())) &&
				req.state == pending {

				mq.reqs[i].block = msg
				break
			}
		}

		// NOTE: Could add logic to check if we ever requested this

	case *RequestMessage:
		mq.reqs = append(mq.reqs, NewBlockRequestStatus(msg))
	default:
		mq.other = append(mq.other, msg)
	}
}

func (mq *MessageQueue) Write(sink *MessageSink) bool {

	// I/O flags
	var request, block, other bool

	// 1. Find request & block to write (if any)
	var newReq, receivedReq *BlockRequestStatus
	var index int
	for i, req := range mq.reqs {
		if req.state == received && receivedReq == nil {
			receivedReq = req
			index = i
		}
		if req.state == new && newReq == nil {
			newReq = req
		}
	}

	// 2. Send request and or block
	if newReq != nil {
		request = mq.WriteRequest(newReq, sink)
	}
	if receivedReq != nil {
		block = mq.WriteBlock(index, receivedReq, sink)
	}

	// 3. Send general protocol traffic
	if len(mq.other) > 0 {
		other = mq.WriteMessage(mq.other[0], sink)
	}

	// Did we perform some I/O?
	return request || block || other
}

func (mq *MessageQueue) WriteBlock(i int, brs *BlockRequestStatus, sink *MessageSink) bool {
	ok := sink.Write(brs.block)
	if ok {
		mq.reqs = append(mq.reqs[:i], mq.reqs[i+1:]...) // Remove
	}
	return ok
}

func (mq *MessageQueue) WriteRequest(brs *BlockRequestStatus, sink *MessageSink) bool {
	ok := sink.Write(brs.req)
	if ok {
		brs.state = pending
	}
	return ok
}

func (mq *MessageQueue) WriteMessage(msg ProtocolMessage, sink *MessageSink) bool {
	ok := sink.Write(msg)
	if ok {
		mq.other = mq.other[1:]
	}
	return ok
}
