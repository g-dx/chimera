package bittorrent

// ----------------------------------------------------------------------------------
// Wire State - key protocol state
// ----------------------------------------------------------------------------------

const (
	newPos byte = iota
	optimisticPos
	chokedPos
	interestedPos
	chokingPos
	interestingPos
)

// Initial State
//
// | 7      | 6      | 5           | 4       | 3          | 2      | 1          | 0   |
// | Unused | Unused | interesting | choking | interested | choked | optimistic | new |
// | 0      | 0      | 0           | 1       | 0          | 1      | 0          | 1   |
const ws = WireState(0x15)

type WireState byte

func (ws WireState) CanDownload() bool {
	return !ws.IsChoked() && ws.IsInteresting()
}

func (ws WireState) IsOptimistic() bool {
	return testBit(ws, optimisticPos)
}

func (ws WireState) IsChoked() bool {
	return testBit(ws, chokedPos)
}

func (ws WireState) IsInterested() bool {
	return testBit(ws, interestedPos)
}

func (ws WireState) IsNew() bool {
	return testBit(ws, newPos)
}

func (ws WireState) IsChoking() bool {
	return testBit(ws, chokingPos)
}

func (ws WireState) IsInteresting() bool {
	return testBit(ws, interestingPos)
}

// ------------------------------------------------------------
// Remote peer control of wire state
// ------------------------------------------------------------

func (ws WireState) Choked() WireState {
	return setBit(ws, chokedPos)
}

func (ws WireState) NotChoked() WireState {
	return clearBit(ws, chokedPos)
}

func (ws WireState) Interested() WireState {
	return setBit(ws, interestedPos)
}

func (ws WireState) NotInterested() WireState {
	return clearBit(ws, interestedPos)
}

// ------------------------------------------------------------
// Local peer control of wire state
// ------------------------------------------------------------

func (ws WireState) Choking() WireState {
	return setBit(ws, chokingPos)
}

func (ws WireState) NotChoking() WireState {
	return clearBit(ws, chokingPos)
}

func (ws WireState) Interesting() WireState {
	return setBit(ws, interestingPos)
}

func (ws WireState) NotInteresting() WireState {
	return clearBit(ws, interestingPos)
}

func (ws WireState) NotNew() WireState {
	return clearBit(ws, newPos)
}

func (ws WireState) Optimistic() WireState {
	return setBit(ws, optimisticPos)
}

func (ws WireState) NotOptimistic() WireState {
	return clearBit(ws, optimisticPos)
}

// ------------------------------------------------------------
// Bit twiddling functions
// ------------------------------------------------------------

func setBit(ws WireState, i byte) WireState {
	return WireState(byte(ws) | (1 << i))
}

func clearBit(ws WireState, i byte) WireState {
	return WireState(byte(ws) & ^(1 << i))
}

func testBit(ws WireState, i byte) bool {
	return (byte(ws) & (1 << i)) > 0
}
