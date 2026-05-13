package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// First sighting accepts; immediate replay rejects.
func TestJTISet_DetectsReplay(t *testing.T) {
	set := NewJTISet(100, time.Minute)

	if !set.CheckAndAdd("jti-1") {
		t.Fatal("first sighting of jti-1 must be accepted")
	}
	if set.CheckAndAdd("jti-1") {
		t.Fatal("replay of jti-1 must be rejected")
	}
}

// Distinct jtis do not interfere with each other.
func TestJTISet_DistinctJTIsIndependent(t *testing.T) {
	set := NewJTISet(100, time.Minute)

	if !set.CheckAndAdd("jti-a") {
		t.Fatal("jti-a first sighting must be accepted")
	}
	if !set.CheckAndAdd("jti-b") {
		t.Fatal("jti-b first sighting must be accepted (independent of jti-a)")
	}
}

// Concurrent presentations of the same jti — exactly one must be accepted.
// Run with -race to catch any non-atomic Get-then-Add window.
func TestJTISet_ConcurrentSameJTI_ExactlyOneAccepted(t *testing.T) {
	set := NewJTISet(100, time.Minute)

	const goroutines = 50
	var accepted atomic.Int32
	var wg sync.WaitGroup

	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if set.CheckAndAdd("contended-jti") {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := accepted.Load(); got != 1 {
		t.Fatalf("expected exactly 1 acceptance, got %d", got)
	}
}
