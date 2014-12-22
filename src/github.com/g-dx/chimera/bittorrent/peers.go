package bittorrent

type IPeer interface {

	// State
	Id() PeerIdentity
	Statistics() *Statistics
	Counters() *Counters
	State() PeerState2
	Close()

	// Incoming messages
	OnMessage(msg ProtocolMessage) error

	// Outgoing messages
	Choke() error
	Unchoke() error
	Interested() error
	Uninterested() error
	Have(index int) error
	Bitfield(bits BitSet) error
	Cancel(index, begin, length int) error
	Request(index, begin, length int) error
	Block(index, begin int, data []byte) error
}

type Counters map[string]*Counter

type PeerState2 interface {

	// Remote peer state
	Choked(ok bool)
	Interested(ok bool)
	Optimistic(ok bool)

	// Local peer state
	Choking(ok bool)
	Interesting(ok bool)

	IsOptimistic()
	IsNew()
	IsChoked()
	IsChoking()
	IsInteresting()
	IsInterested()
}
