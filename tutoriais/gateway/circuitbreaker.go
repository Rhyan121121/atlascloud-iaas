package main

import (
	"sync"
	"time"
)

type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

type CircuitBreaker struct {
	mu               sync.Mutex
	name             string
	state            CircuitState
	failureCount     int
	failureThreshold int
	cooldownDuration time.Duration
	openedAt         time.Time
}

func NewCircuitBreaker(name string, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: threshold,
		cooldownDuration: cooldown,
	}
}

// AllowRequest verifica se a requisição pode prosseguir.
// Se o circuito estiver OPEN, retorna false e os segundos restantes de Retry-After.
func (cb *CircuitBreaker) AllowRequest() (bool, int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true, 0

	case StateOpen:
		if now.After(cb.openedAt.Add(cb.cooldownDuration)) {
			cb.state = StateHalfOpen
			return true, 0 // Permite requisição de sondagem (probe)
		}
		remaining := int(time.Until(cb.openedAt.Add(cb.cooldownDuration)).Seconds())
		if remaining < 1 {
			remaining = 1
		}
		return false, remaining

	case StateHalfOpen:
		// Em Half-Open, permite probe controlada
		return true, 0

	default:
		return true, 0
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = StateClosed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
		cb.openedAt = time.Now()
	}
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
