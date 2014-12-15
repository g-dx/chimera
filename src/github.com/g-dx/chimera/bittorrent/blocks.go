package bittorrent

import (
    "sync"
    "fmt"
)

var pendingPool = &sync.Pool{
    New: func() interface{} {
        return new(PendingRequest)
    },
}

type PendingRequest struct {
    index, begin int
}

// Represents a collection of all blocks which have been requested.
type ProtocolRequestTimer struct {
    timers map[PeerIdentity]*RequestTimer
    cur    int
    m sync.RWMutex
    onReturn func(int, int)
}

type RequestTimer struct {
    reqs    []*PendingRequest
    timeout int
}

func (t *ProtocolRequestTimer) BlocksWaiting(id PeerIdentity) int {
    return len(t.GetTimer(id).reqs)
}

func (t *ProtocolRequestTimer) CreateTimer(id PeerIdentity) {
    t.m.Lock()
    defer t.m.Unlock()
    t.timers[id] = &RequestTimer{make([]*PendingRequest, 0, 10), t.cur}
}

func (t *ProtocolRequestTimer) AddBlock(id PeerIdentity, index, begin int) {
    timer := t.GetTimer(id)
    req := pendingPool.Get().(*PendingRequest)
    req.begin = begin
    req.index = index
    timer.reqs = append(timer.reqs, req)
    if timer.timeout == 0 {
        timer.timeout = t.cur + 60
    }
}

func (t *ProtocolRequestTimer) RemoveBlock(id PeerIdentity, index, begin int) {
    timer := t.GetTimer(id)
    // Remove request
    for i, req := range timer.reqs {
        if req.index == index && req.begin == begin {
            timer.reqs = append(timer.reqs[:i], timer.reqs[i+1:]...)
            pendingPool.Put(req)
            break
        }
    }
    // Reset timer to 60 seconds
    timer.timeout = t.cur + 60
}

// Ticks by a unit of time. This may cause blocks to timeout and be returned to the
// pool.
func (t *ProtocolRequestTimer) Tick(n int) {
    for _, timer := range t.timers {
        if len(timer.reqs) > 0 && timer.timeout <= n {
            // Timeout situation - return newest requested block
            req := timer.reqs[len(timer.reqs)-1]
            t.onReturn(req.index, req.begin)
            timer.reqs = timer.reqs[:len(timer.reqs)-1]
            pendingPool.Put(req)

            // Reset timer to 60 seconds
            timer.timeout = t.cur + 60
        }
    }
    t.cur = n
}

func (t *ProtocolRequestTimer) Close(id PeerIdentity) {
    for _, req := range t.GetTimer(id).reqs {
        t.onReturn(req.index, req.begin)
    }
    t.m.Lock()
    defer t.m.Unlock()
    delete(t.timers, id)
}

func (t *ProtocolRequestTimer) CloseAll() {
    t.m.Lock()
    defer t.m.Unlock()
    for id, timer := range t.timers {
        for _, req := range timer.reqs {
            t.onReturn(req.index, req.begin)
        }
        delete(t.timers, id)
    }
}

func (t *ProtocolRequestTimer) GetTimer(id PeerIdentity) *RequestTimer {
    t.m.RLock()
    defer t.m.RUnlock()
    if timer, ok := t.timers[id]; ok {
        return timer
    }
    panic(fmt.Sprintf("Peer [%v] not found", id))
}
