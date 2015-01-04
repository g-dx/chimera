package bittorrent

import (
	"sync/atomic"
)

const (
	maxQueuedMessages int = 50
)

// ----------------------------------------------------------------------------------
// Queue
// ----------------------------------------------------------------------------------

type PeerQueue struct {
	out  chan<- ProtocolMessage
	in   chan ProtocolMessage
	done chan struct{}
	choke chan chan []*RequestMessage

	pending []ProtocolMessage
	next ProtocolMessage
	onRequestSent func(int, int)
	reqs int32
}

func NewQueue(out chan ProtocolMessage, f func(int, int)) *PeerQueue {
	q := &PeerQueue{
		out:     out,
		in:      make(chan ProtocolMessage),
		done:    make(chan struct{}),
		choke:   make(chan chan []*RequestMessage),

		pending: make([]ProtocolMessage, 0, maxQueuedMessages),
		onRequestSent : f,
	}
	go q.loop()
	return q
}

func (q *PeerQueue) Add(pm ProtocolMessage) { q.in <- pm }

func (q *PeerQueue) QueuedRequests() int { return int(atomic.LoadInt32(&q.reqs)) }

func (q *PeerQueue) Choke() []*RequestMessage {
	c := make(chan []*RequestMessage)
	q.choke <- c
	reqs := <- c
	close(c)
	return reqs
}

func (q *PeerQueue) Close() []*RequestMessage {
	close(q.done)
	close(q.choke)
	close(q.out)
	reqs := q.onChoke()
	for _, msg := range q.pending {
		msg.Recycle()
	}
	if q.next != nil {
		q.next.Recycle()
	}
	return reqs
}

func (q *PeerQueue) loop() {

	var isRequest bool
	var index, begin int
	for {
		// Configure next message to send
		if q.next == nil {
			index, begin, isRequest = q.maybeEnableSend()
		}

		select {
		case msg := <- q.in:
			q.pending = append(q.pending, msg)
			if _, ok := msg.(*RequestMessage); ok {
				atomic.AddInt32(&q.reqs, 1)
			}
		case q.out <- q.next:
			if isRequest{
				q.onRequestSent(index, begin)
				atomic.AddInt32(&q.reqs, -1)
			}
			q.next = nil
		case _ = <- q.done:
			q.drain()
			return
		case c := <- q.choke:
			q.drain()
			c <- q.onChoke()
		}
	}
}

func (q *PeerQueue) maybeEnableSend() (int, int, bool) {
	var index, begin int
	var isReq bool
	if len(q.pending) > 0 {
		q.next = q.pending[0]
		q.pending = q.pending[1:]

		if req, ok := q.next.(*RequestMessage); ok {
			index = int(req.Index())
			begin = int(req.Begin())
			isReq = true
		}
	}
	return index, begin, isReq
}

func (q *PeerQueue) onChoke() []*RequestMessage {
	reqs := make([]*RequestMessage, 0, 5)
	p := q.pending[:0]

	if req, ok := q.next.(*RequestMessage); ok {
		reqs = append(reqs, req)
		q.next = nil
	}

	for _, msg := range q.pending {
		if req, ok := msg.(*RequestMessage); ok {
			reqs = append(reqs, req)
		} else {
			p = append(p, msg)
		}
	}
	q.pending = p
	return reqs
}

func (q *PeerQueue) drain() {

	for {
		select {
		case msg := <- q.in:
			q.pending = append(q.pending, msg)
		default:
			return
		}
	}
}
