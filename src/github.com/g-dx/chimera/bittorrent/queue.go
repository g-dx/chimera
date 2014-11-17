package bittorrent

type PeerQueue struct {
	
} 

// Called we the peer has been choked. Returns all unsent request messages
func (q * PeerQueue) Choke() []*RequestMessage { return nil}

// Called every heartbeat. All blocks which have timed out will be passed to the handler
// which decides which ones to return and which to allow to continue
func (q * PeerQueue) OnHeartbeat(beat int64) []*RequestMessage { return nil }

// Called when a block arrives
func (q * PeerQueue) OnBlock(index, offset, len uint32) {}

// Returns the number of peer requests
func (q * PeerQueue) NumUnsentRequests() int { return 0}

// Returns the number of requests (sent or PeerQueued)
func (q * PeerQueue) NumRequests() {}