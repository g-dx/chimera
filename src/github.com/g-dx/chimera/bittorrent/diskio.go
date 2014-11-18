package bittorrent

type Disk struct {
	rLimit <-chan struct{}
	wLimit <-chan struct{}

	buffers chan []byte
}

func (disk *Disk) Read(index, begin, len uint32, id PeerIdentity, c chan<- ProtocolMessage) error {

	// Rate limit
	<-disk.rLimit

	// Check valid request

	// Block waiting for disk buffer
	//	buf := <-disk.buffers

	// Grab mutex & add to queue

	return nil
}

func (disk *Disk) Write() {

	// Rate limit
	<-disk.wLimit

}

func (disk *Disk) Cancel(index, begin, len uint32) {

}
