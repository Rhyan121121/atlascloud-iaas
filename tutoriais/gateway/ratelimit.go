package main

import (
	"strings"
	"sync"
	"time"
)

type APIKeyProfile struct {
	ClientID        string
	Name            string
	Limit           int // Requisições por minuto
	WindowSecs      int
	AllowedPrefixes []string // Vazio = acesso total; preenchido = restrição por rota (403 Forbidden)
}

var ValidAPIKeys = map[string]APIKeyProfile{
	"cli-terraform": {
		ClientID:   "cli-terraform",
		Name:       "Terraform CLI Profile",
		Limit:      120,
		WindowSecs: 60,
	},
	"console-web": {
		ClientID:   "console-web",
		Name:       "Web Console Profile",
		Limit:      40,
		WindowSecs: 60,
	},
	"parceiro-auditoria": {
		ClientID:        "parceiro-auditoria",
		Name:            "External FinOps Auditor Partner",
		Limit:           30,
		WindowSecs:      60,
		AllowedPrefixes: []string{"/v1/budgets", "/api/v1/budgets", "/v1/security-groups", "/api/v1/security-groups"},
	},
}

type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*clientWindow
}

type clientWindow struct {
	count     int
	windowEnd time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]*clientWindow),
	}
}

// CheckRoutePermission verifica se o perfil autenticado tem autorização para acessar o caminho (Exercício 2 / Ponto Extra: 403 Forbidden)
func (p APIKeyProfile) IsRouteAllowed(path string) bool {
	if len(p.AllowedPrefixes) == 0 {
		return true
	}
	for _, prefix := range p.AllowedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"?") {
			return true
		}
	}
	return false
}

// Allow verifica se a chave de API pode efetuar a requisição.
// Retorna (permitido, segundosParaRetry, perfilValido, limiteTotal, restantes, perfil).
func (rl *RateLimiter) Allow(apiKey string) (bool, int, bool, int, int, APIKeyProfile) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	profile, ok := ValidAPIKeys[apiKey]
	if !ok {
		return false, 0, false, 0, 0, APIKeyProfile{}
	}

	now := time.Now()
	win, exists := rl.windows[apiKey]

	if !exists || now.After(win.windowEnd) {
		rl.windows[apiKey] = &clientWindow{
			count:     1,
			windowEnd: now.Add(time.Duration(profile.WindowSecs) * time.Second),
		}
		return true, 0, true, profile.Limit, profile.Limit - 1, profile
	}

	win.count++
	remaining := profile.Limit - win.count
	if remaining < 0 {
		remaining = 0
	}

	if win.count <= profile.Limit {
		return true, 0, true, profile.Limit, remaining, profile
	}

	retryAfter := int(time.Until(win.windowEnd).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter, true, profile.Limit, 0, profile
}
