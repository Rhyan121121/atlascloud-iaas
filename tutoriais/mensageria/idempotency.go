package main

import (
	"sync"
	"time"
)

// IdempotencyStore armazena IDs de eventos processados para evitar duplicidade de e-mails
type IdempotencyStore struct {
	mu      sync.RWMutex
	seenIDs map[string]time.Time
}

func NewIdempotencyStore() *IdempotencyStore {
	store := &IdempotencyStore{
		seenIDs: make(map[string]time.Time),
	}

	// Limpeza periódica de IDs com mais de 24 horas
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			store.mu.Lock()
			threshold := time.Now().Add(-24 * time.Hour)
			for id, t := range store.seenIDs {
				if t.Before(threshold) {
					delete(store.seenIDs, id)
				}
			}
			store.mu.Unlock()
		}
	}()

	return store
}

// MarkIfNew retorna true se for a primeira vez que o ID é visto. Retorna false se for duplicata.
func (s *IdempotencyStore) MarkIfNew(id string) bool {
	if id == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.seenIDs[id]; exists {
		return false
	}
	s.seenIDs[id] = time.Now().UTC()
	return true
}
