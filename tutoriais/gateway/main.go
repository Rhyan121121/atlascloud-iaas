package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"atlascloud/pkg/httputil"
	"atlascloud/pkg/rfc9457"
)

// Listas de permissão de cabeçalhos (Passo 3 do Tutorial de API Gateway e Restrição EARS [R8]: nunca repassar X-API-Key!)
var AllowedClientHeaders = []string{"Content-Type", "Accept", "Accept-Language", "If-None-Match"}
var AllowedServiceHeaders = []string{"Content-Type", "ETag", "Cache-Control", "Retry-After"}

type RouteMatch struct {
	UpstreamBase   string
	PublicPrefix   string
	InternalPrefix string
	CB             *CircuitBreaker
}

type GatewayServer struct {
	computeURL     string
	securityURL    string
	billingURL     string
	graphqlURL     string
	staticDir      string
	timeout        time.Duration
	rateLimiter    *RateLimiter
	computeCB      *CircuitBreaker
	securityCB     *CircuitBreaker
	billingCB      *CircuitBreaker
	graphqlCB      *CircuitBreaker
	aggregator     *DashboardAggregator
	proxyTransport *http.Transport
}

func NewGatewayServer() *GatewayServer {
	computeURL := os.Getenv("COMPUTE_SERVICE_URL")
	if computeURL == "" {
		computeURL = "http://127.0.0.1:8081"
	}
	securityURL := os.Getenv("SECURITY_SERVICE_URL")
	if securityURL == "" {
		securityURL = "http://127.0.0.1:8082"
	}
	billingURL := os.Getenv("BILLING_SERVICE_URL")
	if billingURL == "" {
		billingURL = "http://127.0.0.1:8083"
	}
	graphqlURL := os.Getenv("GRAPHQL_SERVICE_URL")
	if graphqlURL == "" {
		graphqlURL = "http://127.0.0.1:8084"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./frontend"
	}

	timeoutMs, err := strconv.Atoi(os.Getenv("UPSTREAM_TIMEOUT_MS"))
	if err != nil || timeoutMs <= 0 {
		timeoutMs = 2000 // 2 segundos estritos conforme Restrição EARS [R5] e Tutorial de API Gateway
	}

	return &GatewayServer{
		computeURL:  computeURL,
		securityURL: securityURL,
		billingURL:  billingURL,
		graphqlURL:  graphqlURL,
		staticDir:   staticDir,
		timeout:     time.Duration(timeoutMs) * time.Millisecond,
		rateLimiter: NewRateLimiter(),
		computeCB:   NewCircuitBreaker("compute", 3, 8*time.Second),
		securityCB:  NewCircuitBreaker("security", 3, 8*time.Second),
		billingCB:   NewCircuitBreaker("billing", 3, 8*time.Second),
		graphqlCB:   NewCircuitBreaker("graphql", 3, 8*time.Second),
		aggregator:  NewDashboardAggregator(computeURL, securityURL, billingURL),
		proxyTransport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func (gw *GatewayServer) matchRoute(path string) *RouteMatch {
	switch {
	case strings.HasPrefix(path, "/api/v1/instances"):
		return &RouteMatch{UpstreamBase: gw.computeURL, PublicPrefix: "/api/v1/instances", InternalPrefix: "/v1/instances", CB: gw.computeCB}
	case strings.HasPrefix(path, "/v1/instances"):
		return &RouteMatch{UpstreamBase: gw.computeURL, PublicPrefix: "/v1/instances", InternalPrefix: "/v1/instances", CB: gw.computeCB}
	case strings.HasPrefix(path, "/api/v1/security-groups"):
		return &RouteMatch{UpstreamBase: gw.securityURL, PublicPrefix: "/api/v1/security-groups", InternalPrefix: "/v1/security-groups", CB: gw.securityCB}
	case strings.HasPrefix(path, "/v1/security-groups"):
		return &RouteMatch{UpstreamBase: gw.securityURL, PublicPrefix: "/v1/security-groups", InternalPrefix: "/v1/security-groups", CB: gw.securityCB}
	case strings.HasPrefix(path, "/api/v1/budgets"):
		return &RouteMatch{UpstreamBase: gw.billingURL, PublicPrefix: "/api/v1/budgets", InternalPrefix: "/v1/budgets", CB: gw.billingCB}
	case strings.HasPrefix(path, "/v1/budgets"):
		return &RouteMatch{UpstreamBase: gw.billingURL, PublicPrefix: "/v1/budgets", InternalPrefix: "/v1/budgets", CB: gw.billingCB}
	case strings.HasPrefix(path, "/api/graphql"):
		return &RouteMatch{UpstreamBase: gw.graphqlURL, PublicPrefix: "/api/graphql", InternalPrefix: "/graphql", CB: gw.graphqlCB}
	case strings.HasPrefix(path, "/graphql"):
		return &RouteMatch{UpstreamBase: gw.graphqlURL, PublicPrefix: "/graphql", InternalPrefix: "/graphql", CB: gw.graphqlCB}
	default:
		return nil
	}
}

func (gw *GatewayServer) authenticateAndAuthorize(w http.ResponseWriter, r *http.Request) (APIKeyProfile, bool) {
	apiKey := r.Header.Get("X-API-Key")
	allowed, retryAfter, validKey, limit, remaining, profile := gw.rateLimiter.Allow(apiKey)
	if !validKey {
		rfc9457.WriteProblem(w, r, http.StatusUnauthorized,
			"https://atlascloud.io/errors/unauthorized",
			"Autenticação Obrigatória",
			"Informe uma chave válida no cabeçalho X-API-Key ('cli-terraform', 'console-web' ou 'parceiro-auditoria').")
		return APIKeyProfile{}, false
	}

	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

	// Autorização grossa por rota no Gateway (Exercício 2 / Ponto Extra: 403 Forbidden)
	if !profile.IsRouteAllowed(r.URL.Path) {
		rfc9457.WriteProblem(w, r, http.StatusForbidden,
			"https://atlascloud.io/errors/forbidden",
			"Acesso Negado para Esta Rota",
			fmt.Sprintf("O cliente '%s' não possui permissão para acessar a rota '%s'.", profile.ClientID, r.URL.Path))
		return APIKeyProfile{}, false
	}

	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		rfc9457.WriteProblem(w, r, http.StatusTooManyRequests,
			"https://atlascloud.io/errors/rate-limit-exceeded",
			"Taxa Limite Excedida",
			fmt.Sprintf("Limite de %d requisições por minuto excedido para o plano '%s'. Tente novamente em %d segundos.", limit, profile.ClientID, retryAfter))
		return APIKeyProfile{}, false
	}

	return profile, true
}

func (gw *GatewayServer) forwardRequest(w http.ResponseWriter, r *http.Request, rm *RouteMatch, profile APIKeyProfile) {
	reqID := httputil.GetRequestID(r)

	// 1. Verificação do Circuit Breaker (Disjuntor)
	allowed, retryAfter := rm.CB.AllowRequest()
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		rfc9457.WriteProblem(w, r, http.StatusServiceUnavailable,
			"https://atlascloud.io/errors/circuit-breaker-open",
			"Serviço Temporariamente Indisponível (Circuit Breaker Ativo)",
			fmt.Sprintf("O disjuntor para este serviço está aberto após 3 falhas consecutivas. Tente novamente em %d segundos.", retryAfter))
		return
	}

	// 2. Reescrita de caminho público -> caminho interno
	suffix := strings.TrimPrefix(r.URL.Path, rm.PublicPrefix)
	internalPath := rm.InternalPrefix + suffix
	targetURL := fmt.Sprintf("%s%s", rm.UpstreamBase, internalPath)
	if r.URL.RawQuery != "" {
		targetURL = fmt.Sprintf("%s?%s", targetURL, r.URL.RawQuery)
	}

	ctx, cancel := context.WithTimeout(r.Context(), gw.timeout)
	defer cancel()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
			"https://atlascloud.io/errors/internal-error",
			"Erro Interno no Gateway",
			err.Error())
		return
	}

	// 3. Repassar somente cabeçalhos da lista de permissão + X-Request-Id + X-Cliente-Id (nunca repassa X-API-Key!)
	for _, h := range AllowedClientHeaders {
		if val := r.Header.Get(h); val != "" {
			proxyReq.Header.Set(h, val)
		}
	}
	proxyReq.Header.Set("X-Request-Id", reqID)
	proxyReq.Header.Set("X-Cliente-Id", profile.ClientID)
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)

	client := &http.Client{
		Transport: gw.proxyTransport,
		Timeout:   gw.timeout,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		rm.CB.RecordFailure()

		// Distinguir entre Timeout (504) e Falha de Conexão (502)
		if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
			rfc9457.WriteProblem(w, r, http.StatusGatewayTimeout,
				"https://atlascloud.io/errors/gateway-timeout",
				"Gateway Timeout",
				fmt.Sprintf("O serviço interno não respondeu dentro do limite estrito de %v (Restrição EARS [R5]).", gw.timeout))
			return
		}

		rfc9457.WriteProblem(w, r, http.StatusBadGateway,
			"https://atlascloud.io/errors/bad-gateway",
			"Bad Gateway",
			fmt.Sprintf("Não foi possível obter resposta do serviço interno em '%s'.", rm.UpstreamBase))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		rm.CB.RecordFailure()
	} else {
		rm.CB.RecordSuccess()
	}

	// 4. Repassar somente cabeçalhos de resposta permitidos
	for _, h := range AllowedServiceHeaders {
		if val := resp.Header.Get(h); val != "" {
			w.Header().Set(h, val)
		}
	}

	// 5. Reescrever cabeçalho Location (caminho interno -> caminho público do Gateway)
	if loc := resp.Header.Get("Location"); loc != "" {
		if strings.HasPrefix(loc, rm.InternalPrefix) {
			rewrittenLoc := rm.PublicPrefix + strings.TrimPrefix(loc, rm.InternalPrefix)
			w.Header().Set("Location", rewrittenLoc)
		} else {
			w.Header().Set("Location", loc)
		}
	}

	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	gw := NewGatewayServer()

	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. Verificação de saúde do próprio Gateway (/healthz e /saude fora da autenticação)
		if path == "/healthz" || path == "/saude" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"UP","service":"atlas-gateway"}`))
			return
		}

		// 2. Rota Agregada com Degradação Graciosa: GET /api/v1/dashboard/{accountId} (e /api/painel/dashboard/{accountId})
		if strings.HasPrefix(path, "/api/v1/dashboard/") || strings.HasPrefix(path, "/api/painel/dashboard/") {
			if _, ok := gw.authenticateAndAuthorize(w, r); !ok {
				return
			}
			accountID := strings.TrimPrefix(path, "/api/v1/dashboard/")
			accountID = strings.TrimPrefix(accountID, "/api/painel/dashboard/")
			reqID := httputil.GetRequestID(r)
			data := gw.aggregator.GetDashboard(r.Context(), accountID, reqID)

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", reqID)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(data)
			return
		}

		// 3. Roteamento para APIs Internas (/api/v1/*, /v1/*, /api/graphql, /graphql)
		if rm := gw.matchRoute(path); rm != nil {
			profile, ok := gw.authenticateAndAuthorize(w, r)
			if !ok {
				return
			}
			gw.forwardRequest(w, r, rm, profile)
			return
		}

		// 4. Qualquer rota /api/* inexistente devolve 404 Problem Details gerado pelo Gateway
		if strings.HasPrefix(path, "/api/") {
			rfc9457.WriteProblem(w, r, http.StatusNotFound,
				"https://atlascloud.io/errors/route-not-found",
				"Rota Não Encontrada no Gateway",
				fmt.Sprintf("Nenhuma rota atende %s %s no AtlasCloud Gateway.", r.Method, path))
			return
		}

		// 5. Servir Frontend Estático diretamente na raiz (/)
		cleanPath := filepath.Clean(path)
		targetFile := filepath.Join(gw.staticDir, cleanPath)
		fileInfo, err := os.Stat(targetFile)
		if err == nil && !fileInfo.IsDir() {
			http.ServeFile(w, r, targetFile)
			return
		}

		indexFile := filepath.Join(gw.staticDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			http.ServeFile(w, r, indexFile)
			return
		}

		http.NotFound(w, r)
	})

	handler := httputil.RequestIDMiddleware("atlas-gateway", mainHandler)

	serverAddr := ":" + port
	log.Printf("[atlas-gateway] Ponto único de entrada rodando na porta %s (Proxy + Static Frontend)...", port)
	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[atlas-gateway] Falha fatal no servidor: %v", err)
	}
}
