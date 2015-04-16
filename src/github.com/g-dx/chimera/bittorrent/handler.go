package bittorrent

// Handler functions
type OutgoingMessagesHandler func ([]ProtocolMessage, *Peer)
type IncomingMessagesHandler func ([]ProtocolMessage, *Peer, *PieceMap) (error, []ProtocolMessage, []DiskMessage, []int64)

// Updates the peer in preparation for sending a message
func OnSendMessages(msgs []ProtocolMessage, p *Peer, mp *PieceMap) AddMessages {

	var ext []ProtocolMessage
    for _, msg := range msgs {
        switch m := msg.(type) {
            case Choke: p.ws = p.ws.Choking()
            case Unchoke: p.ws = p.ws.NotChoking()
            case Uninterested: p.ws = p.ws.NotInterested()
			case Cancel:
                p.blocks.Remove(toOffset(m.index, m.begin, mp.pieceSize))
			case Request:
				mp.SetBlock(m.index, m.begin, REQUESTED)
				p.blocks.Add(toOffset(m.index, m.begin, mp.pieceSize))
			case Have:
				// TODO: Check if we are not interested anymore
            case Bitfield, Block, KeepAlive, Interested:
            // No peer state change at present
            default:
            // No peer state change at present
        }
    }

	return AddMessages(append(msgs, ext...))
}

// Updates the peer in response to receiving a message
func OnReceiveMessages(msgs []ProtocolMessage, p *Peer, mp *PieceMap) (error, []ProtocolMessage, []DiskMessage) {

	// State to build during message processing
	var out []ProtocolMessage
	var ops []DiskMessage
	var blocks set
	var err error
	ws := p.ws
	bf := p.bitfield

	for _, msg := range msgs {

		var pm ProtocolMessage
		var op DiskMessage

		// Handle message
		switch m := msg.(type) {
		case Choke: ws, blocks = onChoke(ws, p.blocks, mp)
		case Unchoke: ws = onUnchoke(ws)
		case Interested: ws = onInterested(ws)
		case Uninterested: ws = onUninterested(ws)
		case Bitfield: err, bf, ws, pm = onBitfield([]byte(m), ws, mp)
		case Have: err, ws, pm = onHave(int(m), ws, bf, mp)
		case Cancel: err = onCancel(m.index, m.begin, m.length, mp)
		case Request: err, op = onRequest(m.index, m.begin, m.length, mp, p.id)
		case Block: err, op = onBlock(m.index, m.begin, m.block, mp, p.statistics)
		case KeepAlive: // Nothing to do...
		default:
			//p.logger.Printf("Unknown protocol message: %v", m) TODO: pass a logger
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
    p.blocks = blocks
	return nil, out, ops
}

func onChoke(ws WireState, blocks set, mp *PieceMap) (WireState, set) {
    mp.ReturnOffsets(blocks)
	return ws.Choked(), make(set)
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
		return newError("Invalid Have: %v", index), ws, nil
	}

	var msg ProtocolMessage
	if !bitfield.Have(index) {

		// Update bitfield & update availability
		bitfield.Set(index)
		mp.Inc(index)

		if isNowInteresting(index, ws, mp) {
			ws = ws.Interesting()
			msg = Interested{}
		}
	}
	return nil, ws, msg
}

func onCancel(index, begin, length int, mp *PieceMap) error {

	if !mp.IsValid(index, begin, length) {
		return newError("Cancel Invalid: index(%v), begin(%v), len(%v)", index, begin, length)
	}

	// TODO: implement Cancel functionality for disk!
	// TODO: Support BufferMessage cancel here...
	return nil
}

func onRequest(index, begin, length int, mp *PieceMap, id PeerIdentity) (error, DiskMessage) {

	// Check request valid
	if !mp.IsValid(index, begin, length) {
		return newError("Request Invalid: index(%v), begin(%v), len(%v)", index, begin, length), nil
	}

	// Return message to fill
	return nil, ReadMessage{id, Block{index, begin, make([]byte, 0, length)}}
}

func onBlock(index, begin int, block []byte, mp *PieceMap, s *Statistics) (error, DiskMessage) {

	// Check request valid
	if !mp.IsValid(index, begin, len(block)) {
		return newError("Block Invalid: index(%v), begin(%v), len(%v)", index, begin, len(block)), nil
	}

	s.Download.Add(len(block))
	return nil, WriteMessage(Block{index, begin, block})
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
			ws = ws.Interesting()
			msg = Interested{}
			break
		}
	}
	return nil, bitfield, ws, msg
}

func isNowInteresting(index int, ws WireState, mp *PieceMap) bool {
	return !ws.IsInteresting() && mp.Piece(index).RequestsRequired()
}


