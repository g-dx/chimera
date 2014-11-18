package bittorrent

import "time"

func NewRateLimiter(rps, burst int) chan<- struct{} {

	// Create & fill pipe with burst tokens
	c := make(chan struct{}, burst)
	for i := 1; i <= burst; i++ {
		c <- struct{}{}
	}

	// Start ticker and goroutine to add tokens
	ticker := time.Tick(time.Second / time.Duration(rps))
	go func() {
		for {
			_ = <-ticker
			c <- struct{}{}
		}
	}()
	return c
}
