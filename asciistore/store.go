package asciistore

import (
	"sync"
	"time"
)

type ASCIIStore struct {
	mu    sync.RWMutex
	cache map[string]*ASCIIArt // key: user@network#channel
}

type ASCIIArt struct {
	Lines         []string
	Prompt        string
	User          string
	Network       string
	Channel       string
	Timestamp     time.Time
	UseHalfblocks bool
}

func NewASCIIStore() *ASCIIStore {
	return &ASCIIStore{
		cache: make(map[string]*ASCIIArt),
	}
}

func (s *ASCIIStore) Store(user, network, channel string, lines []string, prompt string, useHalfblocks bool) {
	key := generateKey(user, network, channel)

	art := &ASCIIArt{
		Lines:         lines,
		Prompt:        prompt,
		User:          user,
		Network:       network,
		Channel:       channel,
		Timestamp:     time.Now(),
		UseHalfblocks: useHalfblocks,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = art
}

func (s *ASCIIStore) Retrieve(user, network, channel string) (*ASCIIArt, bool) {
	key := generateKey(user, network, channel)

	s.mu.RLock()
	defer s.mu.RUnlock()
	art, exists := s.cache[key]
	return art, exists
}

func (s *ASCIIStore) Clear(user, network, channel string) {
	key := generateKey(user, network, channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, key)
}

func generateKey(user, network, channel string) string {
	return user + "@" + network + "#" + channel
}
