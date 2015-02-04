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
		{[]ProtocolMessage{ Choke(), Unchoke(), Interested(), Uninterested(), Have(1) }},
		// Send all same messages
		{[]ProtocolMessage{ Choke(), Choke(), Choke() }},
		// Send invalid message
		{[]ProtocolMessage{ &GenericMessage{} }},
	}

	// Create buffer & ensure
	in, out := Buffer(0)
	defer func() { in <- CloseMessage{} }()

	for i, tt := range addRemoveTests {
		for _, msg := range tt.msgs {
			// Send and receive
			in <- msg
			actual := <- out
			if msg != actual {
				t.Errorf(errorString, i, ToString(msg), ToString(actual))
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
		{[]ProtocolMessage{ Choke(), Have(1), Unchoke(), Interested(), Uninterested() },
			func(pm ProtocolMessage) bool { return pm.Id() == haveId },
		 []ProtocolMessage{ Choke(), Unchoke(), Interested(), Uninterested() }},

		// Filter all message of type
		{[]ProtocolMessage{ Choke(), Choke(), Interested(), Choke() },
			func(pm ProtocolMessage) bool { _, ok := pm.(*ChokeMessage); return ok },
		 []ProtocolMessage{ Interested() }},

		// Filter match nothing
		{[]ProtocolMessage{ Request(1, 2, 3), Cancel(4, 5, 6), Unchoke(), Uninterested() },
			func(pm ProtocolMessage) bool { _, ok := pm.(*BitfieldMessage); return ok },
		 []ProtocolMessage{ Request(1, 2, 3), Cancel(4, 5, 6), Unchoke(), Uninterested() }},

		// Filter match first message sent
		{[]ProtocolMessage{ Request(7, 8, 9), Interested(), Unchoke(), Uninterested() },
			func(pm ProtocolMessage) bool { return pm.Id() == requestId },
		 []ProtocolMessage{ Interested(), Unchoke(), Uninterested() }},
	}

	// Run tests
	for n, tt := range filterTests {
		// Create buffer
		in, out := Buffer(0)
		// Send
		for _, msg := range tt.msgs { in <- msg }
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
		if ToString(msg) != ToString(actual[i]) {
			return false
		}
	}
	return true
}


var id = &PeerIdentity{}

var addTests = []struct {
	msg ProtocolMessage
	reqs int
}{
	{KeepAlive, 2},
	{Choke(), 2},
	{Unchoke(), 2},
	{Interested(), 2},
	{Uninterested(), 2},
	{Have(3), 2},
	{Bitfield(make([]byte, 0, 10)), 2},
	{Request(1, 1, 1), 1},
	{Request(2, 2, 2), 0},
	{Cancel(3, 3, 2), 0},
	{Block(3, 3, make([]byte, 0, 10)), 0},
}

func TestQueueAdd(t *testing.T) {
    out := make(chan ProtocolMessage)
	q := NewQueue(out, func(int, int) {})
	defer q.Close()

	// Add all messages
	for _, tt := range addTests {
		q.Add(tt.msg)
	}

	// Receive all messages
	for _, tt := range addTests {
		actual := <- out
		if tt.msg != actual {
			t.Errorf("\nExpected: %v\nActual  : %v", ToString(tt.msg), ToString(actual))
		}
		// TODO: Add this test back in when we can find a way to do this reliably
//		reqs := q.QueuedRequests()
//		if tt.reqs != reqs {
//			t.Errorf("\nExpected: %v\nActual  : %v", tt.reqs, reqs)
//		}
	}
}

var chokeTests = []RequestList {
	asMessageList(
		Choke(),
		Unchoke(),
		Interested(),
	),
	asMessageList(
		KeepAlive,
		Request(1, 2, 3),
		Choke(),
		Unchoke(),
		Request(4, 5, 6),
		Interested(),
		Request(7, 8, 9),
	),
	asMessageList(
		Request(10, 11, 12),
		Request(13, 14, 15),
		Request(16, 17, 18),
	),
	asMessageList(
		KeepAlive,
		Choke(),
		Unchoke(),
		Request(19, 20, 21),
		Request(22, 23, 24),
		Request(25, 26, 27),
		KeepAlive,
	),
}

func TestQueueChoke(t *testing.T) {
	out := make(chan ProtocolMessage)
	q := NewQueue(out, func(int, int) {})
	defer q.Close()

	for run, tt := range chokeTests {
		// Send all messages
		for _, msg := range tt.allMsgs {
			q.Add(msg)
		}

		// Check requests from choke
		reqs := q.Choke()
		for i, expected := range tt.allReqs {
			actual := reqs[i]
			if expected != actual {
				t.Errorf("\nRow: %v\nExpected: %v\nActual  : %v", run, ToString(expected), ToString(actual))
			}
		}

		// Check remaining messages
		for _, expected := range tt.remaining {
			actual := <- out
			if expected != actual {
				t.Errorf("\nRow: %v\nExpected: %v\nActual  : %v", run, ToString(expected), ToString(actual))
			}
		}
	}
}

func BenchmarkQueueAddNoOutput(b *testing.B) {
	q := NewQueue(make(chan ProtocolMessage), func(int, int) {})
	defer q.Close()
	for i := 0; i < b.N; i++ {
		q.Add(Cancel(1, 2, 3))
	}
}

func BenchmarkQueueAddWithOutput(b *testing.B) {
	b.StopTimer()
	out := make(chan ProtocolMessage)
	q := NewQueue(out, func(int, int) {})

	// Setup goroutine to dump messages from queue
	dump := func() {
		for {
			_ = <- out
		}
	}
	go dump()

	msgs := asMessageList(
		KeepAlive,
		Choke(),
		Unchoke(),
		Request(19, 20, 21),
		Cancel(1, 2, 3),
		Request(25, 26, 27),
		KeepAlive,
	)
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		q.Add(msgs.allMsgs[i%len(msgs.allMsgs)])
	}
	// TODO: Why does this screw up the benchmark?
	//	b.StopTimer()
	//	q.Close()
	//	b.StartTimer()
}

type RequestList struct {
	allMsgs []ProtocolMessage
	allReqs []*RequestMessage
	remaining []ProtocolMessage
}

func asMessageList(pms ...ProtocolMessage) RequestList {
	allMsgs := make([]ProtocolMessage, 0, 10)
	allReqs := make([]*RequestMessage, 0, 10)
	remaining := make([]ProtocolMessage, 0, 10)
	for _, msg := range pms {
		allMsgs = append(allMsgs, msg)
		if req, ok := msg.(*RequestMessage); ok {
			allReqs = append(allReqs, req)
		} else {
			remaining = append(remaining, msg)
		}
	}
	return RequestList{allMsgs, allReqs, remaining}
}
