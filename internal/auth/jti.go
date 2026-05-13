package auth

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// JTISet tracks recently-seen JWT IDs to detect replay attacks.
//
// CheckAndAdd is atomic — concurrent callers presenting the same jti
// will see exactly one acceptance.
type JTISet struct {
	mu    sync.Mutex
	cache *expirable.LRU[string, struct{}]
}

func NewJTISet(maxEntries int, ttl time.Duration) *JTISet {
	return &JTISet{
		cache: expirable.NewLRU[string, struct{}](maxEntries, nil, ttl),
	}
}

// CheckAndAdd returns true if this is the first sighting of jti
// within the cache window. Returns false on replay.
func (s *JTISet) CheckAndAdd(jti string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, seen := s.cache.Get(jti); seen {
		return false
	}
	s.cache.Add(jti, struct{}{})
	return true
}
