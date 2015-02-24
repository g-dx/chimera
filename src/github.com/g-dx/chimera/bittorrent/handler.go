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
		case Cancel: err = onCancel(m.index, m.begin, m.length)
		case Request: err, op = onRequest(m.index, m.begin, m.length)
		case Block: err, op = onBlock(m.index, m.begin, m.block, p.statistics, p.id)
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
		return newError("Invalid index received: %v", index), ws, nil
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

func onCancel(index, begin, length int) error {
    // TODO: Return FilterMessage
    // TODO: Return CancelDiskRead
	return nil
}

func onRequest(index, begin, length int) (error, DiskMessage) {

	// TODO: Check request valid
	//	p.pieceMap.IsValid()

	// Get block message and pass to disk to fill
	//	p.disk.Read(index, begin, p.id)
	return nil, nil
}

func onBlock(index, begin int, block []byte, s *Statistics, id PeerIdentity) (error, DiskMessage) {
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


