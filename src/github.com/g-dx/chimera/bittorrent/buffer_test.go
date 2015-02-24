package bittorrent

import (
	"testing"
)

const errorString = "\nRun     : %v\nExpected: %v\nActual  : %v"

func TestBufferAddRemove(t *testing.T) {

	// Create test data
	addRemoveTests := []struct {
		msgs []ProtocolMessage
	}{
		// Send different messsages
		{[]ProtocolMessage{ Choke{}, Unchoke{}, Interested{}, Uninterested{}, Have(1) }},
		// Send all same messages
		{[]ProtocolMessage{ Choke{}, Choke{}, Choke{} }},
	}

	// Create buffer & ensure
	in, out := Buffer(0)
	defer func() { in <- CloseMessage{} }()

	// Test single add + remove
	for i, tt := range addRemoveTests {
		for _, expected := range tt.msgs {
			// Send and receive
			in <- AddMessage(expected)
			actual := <- out
			if expected != actual {
				t.Errorf(errorString, i, ToString(expected), ToString(actual))
			}
		}
	}

	// Test bulk add + remove
	for i, tt := range addRemoveTests {
		// Send all
		in <- AddMessages(tt.msgs)
		for _, expected := range tt.msgs {
			actual := <- out
			if expected != actual {
				t.Errorf(errorString, i, ToString(expected), ToString(actual))
			}
		}
	}
}

func TestBufferFilter(t *testing.T) {

	// Create test data
	filterTests := []struct {
		msgs []ProtocolMessage
		f func(ProtocolMessage) bool
	    expected []ProtocolMessage
	}{
		// Filter specific message
		{[]ProtocolMessage{ Choke{}, Have(1), Unchoke{}, Interested{}, Uninterested{} },
			func(pm ProtocolMessage) bool { return Id(pm) == haveId },
		 []ProtocolMessage{ Choke{}, Unchoke{}, Interested{}, Uninterested{} }},

		// Filter all message of type
		{[]ProtocolMessage{ Choke{}, Choke{}, Interested{}, Choke{} },
			func(pm ProtocolMessage) bool { _, ok := pm.(Choke); return ok },
		 []ProtocolMessage{ Interested{} }},

		// Filter match nothing
		{[]ProtocolMessage{ Request{1, 2, 3}, Cancel{4, 5, 6}, Unchoke{}, Uninterested{} },
			func(pm ProtocolMessage) bool { _, ok := pm.(Bitfield); return ok },
		 []ProtocolMessage{ Request{1, 2, 3}, Cancel{4, 5, 6}, Unchoke{}, Uninterested{} }},

		// Filter match first message sent
		{[]ProtocolMessage{ Request{7, 8, 9}, Interested{}, Unchoke{}, Uninterested{} },
			func(pm ProtocolMessage) bool { return Id(pm) == requestId },
		 []ProtocolMessage{ Interested{}, Unchoke{}, Uninterested{} }},
	}

	// Run tests
	for n, tt := range filterTests {
		// Create buffer
		in, out := Buffer(0)
		// Send
		in <- AddMessages(tt.msgs)
		// Filter
		in <- FilterMessage(tt.f)
		// Receive
		var actual []ProtocolMessage
		for i := 0; i < len(tt.expected); i++ {
			actual = append(actual, <-out)
		}
		if !messagesEqual(tt.expected, actual, t) {
			t.Errorf(errorString, n, tt.expected, actual)
		}
		// Check no other messages
		var add []ProtocolMessage
		select {
		case msg := <-out: add = append(add, msg)
		default:
		}
		if len(add) != 0 {
			t.Errorf(errorString, n, []ProtocolMessage{}, add)
		}
		// Close
		in <- CloseMessage{}
	}
}

func messagesEqual(expected []ProtocolMessage, actual []ProtocolMessage, t *testing.T) bool {
	// Check length
	if len(expected) != len(actual) {
		return false
	}
	// Check contents
	for i, msg := range expected {
		if msg != actual[i] {
			return false
		}
	}
	return true
}