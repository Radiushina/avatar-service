package broker

import (
	"testing"
	"testing/synctest"
)

func TestWatchQueueDepthExits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stop := make(chan struct{})
		go (&Client{}).WatchQueueDepth(stop)
		synctest.Wait()
		close(stop)
	})
}
