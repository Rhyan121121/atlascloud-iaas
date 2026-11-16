package httputil

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const (
	RequestIDHeader             = "X-Request-Id"
	RequestIDContextKey contextKey = "request_id"
)

// responseWriterInterceptor intercepta o status code para log
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// RequestIDMiddleware assegura e propaga o cabeçalho X-Request-Id
func RequestIDMiddleware(serviceName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := r.Header.Get(RequestIDHeader)
		if reqID == "" {
			if id, err := uuid.NewV7(); err == nil {
				reqID = id.String()
			} else {
				reqID = uuid.NewString()
			}
		}

		ctx := context.WithValue(r.Context(), RequestIDContextKey, reqID)
		r = r.WithContext(ctx)

		w.Header().Set(RequestIDHeader, reqID)

		interceptor := &responseWriterInterceptor{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(interceptor, r)

		log.Printf("[%s] [%s] %s %s -> %d (%v)",
			serviceName, reqID, r.Method, r.URL.Path, interceptor.statusCode, time.Since(start))
	})
}

// GetRequestID extrai o ID da requisição do header ou contexto
func GetRequestID(r *http.Request) string {
	if reqID := r.Header.Get(RequestIDHeader); reqID != "" {
		return reqID
	}
	if reqID, ok := r.Context().Value(RequestIDContextKey).(string); ok && reqID != "" {
		return reqID
	}
	return ""
}
