package bittorrent

import (
	"testing"
	"runtime"
)

var id = &PeerIdentity{}

var addTests = []struct {
	msg ProtocolMessage
	reqs int
}{
	{KeepAlive, 2},
	{Choke(id), 2},
	{Unchoke(id), 2},
	{Interested(id), 2},
	{Uninterested(id), 2},
	{Have(id, 3), 2},
	{Bitfield(id, make([]byte, 0, 10)), 2},
	{Request(id, 1, 1, 1), 1},
	{Request(id, 2, 2, 2), 0},
	{Cancel(id, 3, 3, 2), 0},
	{Block(id, 3, 3, make([]byte, 0, 10)), 0},
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
		// TODO: Fix me! We give up execution to allow queue to update num of requests
		runtime.Gosched()
		reqs := q.QueuedRequests()
		if tt.reqs != reqs {
			t.Errorf("\nExpected: %v\nActual  : %v", tt.reqs, reqs)
		}
	}
}

var chokeTests = []MessageList {
	asMessageList(
		Choke(id),
		Unchoke(id),
		Interested(id),
	),
	asMessageList(
		KeepAlive,
		Request(id, 1, 2, 3),
		Choke(id),
		Unchoke(id),
		Request(id, 4, 5, 6),
		Interested(id),
		Request(id, 7, 8, 9),
	),
	asMessageList(
		Request(id, 10, 11, 12),
		Request(id, 13, 14, 15),
		Request(id, 16, 17, 18),
	),
	asMessageList(
		KeepAlive,
		Choke(id),
		Unchoke(id),
		Request(id, 19, 20, 21),
		Request(id, 22, 23, 24),
		Request(id, 25, 26, 27),
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
		q.Add(Cancel(id, 1, 2, 3))
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
		Choke(id),
		Unchoke(id),
		Request(id, 19, 20, 21),
		Cancel(id, 1, 2, 3),
		Request(id, 25, 26, 27),
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

type MessageList struct {
	allMsgs []ProtocolMessage
	allReqs []*RequestMessage
	remaining []ProtocolMessage
}

func asMessageList(pms ...ProtocolMessage) MessageList {
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
	return MessageList{allMsgs, allReqs, remaining}
}
