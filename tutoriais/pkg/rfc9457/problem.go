package rfc9457

import (
	"encoding/json"
	"net/http"
	"time"
)

// InvalidParam detalha campos com falha de validação em respostas 422
type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ProblemDetails representa o padrão RFC 9457 (Problem Details for HTTP APIs)
type ProblemDetails struct {
	Type          string         `json:"type"`
	Title         string         `json:"title"`
	Status        int            `json:"status"`
	Detail        string         `json:"detail"`
	Instance      string         `json:"instance,omitempty"`
	InvalidParams []InvalidParam `json:"invalid_params,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
}

// WriteProblem escreve uma resposta formatada em application/problem+json
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, problemType, title, detail string, invalidParams ...InvalidParam) {
	reqID := r.Header.Get("X-Request-Id")
	instanceURI := r.URL.Path

	p := ProblemDetails{
		Type:          problemType,
		Title:         title,
		Status:        status,
		Detail:        detail,
		Instance:      instanceURI,
		InvalidParams: invalidParams,
		RequestID:     reqID,
		Timestamp:     time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
