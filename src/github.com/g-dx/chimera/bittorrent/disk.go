package bittorrent

import "log"

func mockDisk(reader chan RequestMessage, writer chan BlockMessage, logger *log.Logger) {
	for {
		select {
		case r := <- reader:
			logger.Printf("Disk Read: %v\n", r)
		case w := <- writer:
			logger.Printf("Disk Write: %v\n", w)
		}
	}
}
