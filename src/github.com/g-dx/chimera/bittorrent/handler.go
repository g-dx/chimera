package bittorrent

// Handler functions
type OutgoingMessagesHandler func ([]ProtocolMessage, *Peer)
type IncomingMessagesHandler func ([]ProtocolMessage, *Peer, *PieceMap) (error, []ProtocolMessage, []DiskMessage, []int64)

// Updates the peer in preparation for sending a message
func OnSendMessages(msgs []ProtocolMessage, p *Peer, mp *PieceMap) {

    for _, msg := range msgs {
        switch m := msg.(type) {
            case Choke: p.ws = p.ws.Choking()
            case Unchoke: p.ws = p.ws.NotChoking()
            case Interested: p.ws = p.ws.Interested()
            case Uninterested: p.ws = p.ws.NotInterested()
            case Cancel: // TODO: Remove from peer blocks
            case Request: p.blocks.Add(toOffset(m.index, m.begin, mp.pieceSize))
            case Bitfield, Have, Block, KeepAlive:
            // No peer state change at present
            default:
            // No peer state change at present
        }
    }
}

// Updates the peer in response to receiving a message
func OnReceiveMessages(msgs []ProtocolMessage, p *Peer, mp *PieceMap) (error, []ProtocolMessage, []DiskMessage, []int64) {

	// State to build during message processing
	var out []ProtocolMessage
	var ops []DiskMessage
	var blocks []int64
	var err error
	ws := p.ws
	bf := p.bitfield

	for _, msg := range msgs {

		var pm ProtocolMessage
		var op DiskMessage

		// Handle message
		switch m := msg.(type) {
		case Choke:
			ws, blocks = onChoke(ws, p.blocks)
			p.blocks = make(set)
		case Unchoke: ws = onUnchoke(ws)
		case Interested: ws = onInterested(ws)
		case Uninterested: ws = onUninterested(ws)
		case Bitfield: err, bf, ws, pm = onBitfield([]byte(m), ws, mp)
		case Have: err, ws, pm = onHave(int(m), ws, bf, mp)
		case Cancel: err = onCancel(m.index, m.begin, m.length)
		case Request: err, op = onRequest(m.index, m.begin, m.length)
		case Block: err, op = onBlock(m.index, m.begin, m.block, p.statistics, p.id)
		case KeepAlive: // Nothing to do...
		default:
			p.logger.Printf("Unknown protocol message: %v", m)
		}

		// Check for error & bail
		if err != nil {
			panic(err)
		}

		// Add outgoing messages
		if pm != nil {
			out = append(out, pm)
		}

		// Add disk ops
		if op != nil {
			ops = append(ops, op)
		}
	}

	// Update peer state
	p.ws = ws
	p.bitfield = bf
	return nil, out, ops, blocks
}

func onChoke(ws WireState, blocks set) (WireState, []int64) {
	ret := make([]int64, len(blocks))
	for block, _ := range blocks {
		ret = append(ret, block)
	}
	return ws.Choked(), ret
}

func onUnchoke(ws WireState) WireState {
	return ws.NotChoked()
}

func onInterested(ws WireState) WireState {
	return ws.Interested()
}

func onUninterested(ws WireState) WireState {
	return ws.NotInterested()
}

func onHave(index int, ws WireState, bitfield *BitSet, mp *PieceMap) (error, WireState, ProtocolMessage) {

	// Validate
	if !bitfield.IsValid(index) {
		return newError("Invalid index received: %v", index), ws, nil
	}

	var msg ProtocolMessage
	if !bitfield.Have(index) {

		// Update bitfield & update availability
		bitfield.Set(index)
		mp.Inc(index)

		if isNowInteresting(index, ws, mp) {
			msg = Interested{}
		}
	}
	return nil, ws, msg
}

func onCancel(index, begin, length int) error {
	// TODO: Implement cancel support
	return nil
}

func onRequest(index, begin, length int) (error, DiskMessage) {

	// TODO: Check request valid
	//	p.pieceMap.IsValid()

	// Get block message and pass to disk to fill
	//	p.disk.Read(index, begin, p.id)
	return nil, nil
}

func onBlock(index, begin int, block []byte, s *Statistics, id *PeerIdentity) (error, DiskMessage) {
	// NOTE: Already on the way to disk...
	// p.disk.Write(index, begin, length, p.id)
	//	p.disk.Write(index, begin, block)
	s.Download.Add(len(block))
	return nil, ReadMessage{id, Block{index, begin, block}}
}

func onBitfield(bits []byte, ws WireState, mp *PieceMap) (error, *BitSet, WireState, ProtocolMessage) {

	// Create & validate bitfield
	bitfield, err := NewBitSetFrom(bits, len(mp.pieces))
	if err != nil {
		return err, nil, ws, nil
	}

	// Update availability in global piece map
	mp.IncAll(bitfield)

	// Check if we are interested
	var msg ProtocolMessage
	for i := 0; i < bitfield.Size(); i++ {
		if mp.Piece(i).RequestsRequired() {
			msg = Interested{}
			break
		}
	}
	return nil, bitfield, ws, msg
}

func isNowInteresting(index int, ws WireState, mp *PieceMap) bool {
	return !ws.IsInteresting() && mp.Piece(index).RequestsRequired()
}


