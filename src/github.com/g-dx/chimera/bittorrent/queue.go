package bittorrent

import (
	"sync"
	"errors"
)

const (
	maxQueuedMessages int = 50
)

var errNoBufferSpace = errors.New("Outgoing buffer full.")

type PeerQueue struct {

	out chan<- ProtocolMessage
	done <-chan struct {}
	signal <-chan struct {}

	next ProtocolMessage
	pending []ProtocolMessage
	unsentRequests int
	sentRequests []*RequestMessage
	timer int64
	mutex sync.Mutex

}

func (q * PeerQueue) Add(pm ProtocolMessage) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if len(q.pending) == maxQueuedMessages {
		return errNoBufferSpace
	}

	// Update count & append to queue
	if isRequestMessage(pm) {
		q.unsentRequests++
	}
	q.pending = append(q.pending, pm)
	q.signalIfNecessary()
	return nil
}

func NewQueue(out chan ProtocolMessage) *PeerQueue {
	return &PeerQueue {
		out : out,
		done : make(chan struct{}),
		signal : make(chan struct{}),
		next : nil,
		pending : make([]ProtocolMessage, 0, maxQueuedMessages),
		unsentRequests : 0,
		sentRequests : make([]ProtocolMessage, 0, maxQueuedMessages),
		timer : 0,
		mutex : new(sync.Mutex),
	}
}

// Called we the peer has been choked. Returns all unsent request messages
func (q * PeerQueue) Choke() []*RequestMessage {
	// Clear all pending request messages...
	return nil
}

// Called every heartbeat. All blocks which have timed out will be passed to the handler
// which decides which ones to return and which to allow to continue
func (q * PeerQueue) OnHeartbeat(beat int64) []*RequestMessage {

	// How do we compare with timer?
	return nil
}

// Called when a block arrives
func (q * PeerQueue) OnBlock(index, offset, len uint32) {
	// Remove from sent queue
	// Reset timer?
}

func (q * PeerQueue) NumRequests() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.sentRequests) + q.unsentRequests
}

func (q * PeerQueue) loop() {

	var c chan ProtocolMessage
	for {
		c, q.next = q.maybeEnableSend()
		select {
		case c <- q.next:
			if isRequestMessage(q.next) {
				q.maybeStartRequestTimer(q.next)
			}
			q.next = nil
		case <- q.done:
			return
		case <- q.signal:
		}
	}
}


func (q * PeerQueue) Clear(p func(ProtocolMessage) bool) []ProtocolMessage {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Collect all cleared requests
	msgs := make([]ProtocolMessage, 10)
	for i, m := range q.pending {
		if p(m) {
			// Remove & append to cleared list
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			msgs = append(msgs, m)
		}
	}
	return msgs
}

func (q *PeerQueue) maybeStartRequestTimer(pm ProtocolMessage) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.unsentRequests--

	// TODO: start timer??

}

func (q *PeerQueue) Close() []*RequestMessage {

	q.done <- struct{} // Blocking...

	// Now collect sent * pending requests. No need for mutex anymore...
	return nil
}

func (q *PeerQueue) maybeEnableSend() (c chan<- *ProtocolMessage, next ProtocolMessage) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Prep next message for sending
	for len(q.pending) > 0 {
		next = q.pending[0]
		c = q.out
	}
	return c, next
}

func isRequestMessage(pm ProtocolMessage) bool {
	_, ok := pm.(*RequestMessage)
	return ok
}

func (p * PeerQueue) signalIfNecessary() {
	// Non-blocking send...
	select {
	case p.signal <- struct {} :
	default:
	}
}
