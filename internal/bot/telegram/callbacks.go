package telegram

import (
	"fmt"
	"sync"
	"time"
)

type callbackRef struct {
	Provider  string
	Account   string
	TargetID  string
	Extra     string
	CreatedAt time.Time
}

// callbackStore maps short base36 tokens to long cloud identifiers (OCIDs, Azure resource IDs)
// so all Telegram callback_data payloads remain strictly <= 64 bytes.
type callbackStore struct {
	mu      sync.Mutex
	counter uint64
	items   map[string]callbackRef
}

func newCallbackStore() *callbackStore {
	return &callbackStore{
		items: make(map[string]callbackRef),
	}
}

func (s *callbackStore) Put(ref callbackRef) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	tok := fmt.Sprintf("k%x", s.counter)
	ref.CreatedAt = time.Now()
	s.items[tok] = ref
	if len(s.items) > 3000 {
		cutoff := time.Now().Add(-12 * time.Hour)
		for k, v := range s.items {
			if v.CreatedAt.Before(cutoff) {
				delete(s.items, k)
			}
		}
		// Hard cap if bursts happen within 12h
		if len(s.items) > 3500 {
			for k := range s.items {
				delete(s.items, k)
				if len(s.items) <= 2000 {
					break
				}
			}
		}
	}
	return tok
}

func (s *callbackStore) Get(tok string) (callbackRef, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.items[tok]
	return ref, ok
}
