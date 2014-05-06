package bittorrent

import "log"

type DiskMessage interface {
	Id() PeerIdentity
}

type WriteMessage struct {
	id PeerIdentity
	index, begin uint32
	block *[]byte
}

func (dw WriteMessage) Id() PeerIdentity {
	return dw.id
}

type ReadMessage struct {
	id PeerIdentity
	index, begin, length uint32
}

func (dm ReadMessage) Id() PeerIdentity {
	return dm.id
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

