package bittorrent

import "log"

type DiskMessage interface {
	Id() PeerIdentity
}

type DiskWriteMessage struct {
	id PeerIdentity
	index, begin uint32
	block []byte
}

func (dw DiskWriteMessage) Id() PeerIdentity {
	return dw.id
}

func DiskWrite(b *BlockMessage, id PeerIdentity) *DiskWriteMessage {
	return &DiskWriteMessage {
		id : id,
		index : b.Index(),
		begin : b.Begin(),
		block : b.Block(),
	}
}

type DiskReadMessage struct {
	id PeerIdentity
	index, begin, length uint32
}

func (dm DiskReadMessage) Id() PeerIdentity {
	return dm.id
}

func DiskRead(r *RequestMessage, id PeerIdentity) *DiskReadMessage {
	return &DiskReadMessage {
		id : id,
		index : r.Index(),
		begin : r.Begin(),
		length : r.Length(),
	}
}

func mockDisk(reader <-chan DiskMessage, logger *log.Logger) {
	for {
		select {
		case r := <- reader:
			logger.Printf("Disk Read: %v\n", r)
//		case w := <- writer:
//			logger.Printf("Disk Write: %v\n", w)
		}
	}
}

