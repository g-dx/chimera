package bittorrent

type RingBuffer struct {
	buffer chan ProtocolMessage
	cap, size int
}

func NewRingBuffer(cap int) *RingBuffer {
	return &RingBuffer {
		buffer : make(chan ProtocolMessage, cap),
		cap : cap,
	}
}

func (rb * RingBuffer) IsFull() bool {
	return rb.size == rb.cap
}

func (rb * RingBuffer) IsEmpty() bool {
	return rb.size == 0
}

func (rb * RingBuffer) Next() ProtocolMessage {
	if rb.IsEmpty() {
		panic(newError("Buffer underflow."))
	}
	msg := <- rb.buffer
	rb.size--
	return msg
}

func (rb * RingBuffer) Add(msg ProtocolMessage) {
	if rb.IsFull() {
		panic(newError("Buffer overflow."))
	}
	rb.buffer <- msg
	rb.size++
}

func (rb * RingBuffer) Fill(in <-chan ProtocolMessage) {
	for !rb.IsFull() {
		select {
			case msg := <- in: rb.Add(msg)
			default: break
		}
	}
}

func (rb * RingBuffer) Flush(out chan<- ProtocolMessage) int {
	n := 0
	for !rb.IsEmpty() {
		select {
			case out <- rb.Next(): n++
			default: break
		}
	}
	return n
}

