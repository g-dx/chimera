package bittorrent

import (
	"fmt"
)

// ----------------------------------------------------------------------------------
// Buffer
// ----------------------------------------------------------------------------------

// Supported message types
type BufferMessage interface {}
type AddMessage ProtocolMessage
type AddMessages []ProtocolMessage
type FilterMessage func(ProtocolMessage) bool
type CloseMessage struct{}

// Creates a new buffer of the given size. Returns a channel to control
// the buffer & a downstream channel.
func Buffer(size int) (chan<- BufferMessage, chan ProtocolMessage) {
	in := make(chan BufferMessage, size)
	out := make(chan ProtocolMessage)
	go bufferImpl(in, out)
	return in, out
}

func bufferImpl(in chan BufferMessage, out chan ProtocolMessage) {

	var pending []ProtocolMessage
	for {

		// Enable send when pending non-empty
		var next ProtocolMessage
		var c chan<- ProtocolMessage
		if len(pending) > 0 {
			next = pending[0]
			c = out
		}

		select {

		// Upstream receive
		case msg := <- in:

			switch m := msg.(type) {
			case CloseMessage:
				close(in)
				close(out)
				return

			case FilterMessage:
				tmp := pending[:0]
				for _, msg := range pending {
					if !m(msg) {
						tmp = append(tmp, msg)
					}
				}
				pending = tmp

			case AddMessages:
				for _, msg := range m {
					pending = append(pending, msg)
				}

			case AddMessage:
				pending = append(pending, m)

			default:
				panic(fmt.Sprintf("Unknown buffer message: %v", msg))
			}

		// Downstream send
		case c <- next:
			pending = pending[1:]
		}
	}
}