package bittorrent

func loop() {

	out chan<- ProtocolMessage
	in chan<- ProtocolMessage
	go incoming.Run(out)
	go outgoing.Run(in)
}
