package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"atlascloud/pkg/httputil"
	"atlascloud/pkg/rfc9457"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}
	computeURL := os.Getenv("COMPUTE_SERVICE_URL")
	if computeURL == "" {
		computeURL = "http://127.0.0.1:8081"
	}
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	service := NewBillingService(computeURL, rabbitURL, "atlas.events")

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"atlas-billing-service"}`))
	})

	// GET /v1/budgets/{accountId}
	mux.HandleFunc("GET /v1/budgets/{accountId}", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		reqID := httputil.GetRequestID(r)

		info, err := service.GetBudget(r.Context(), accountID, reqID)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
				"https://atlascloud.io/errors/internal-error",
				"Erro no Faturamento",
				err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	// POST /v1/budgets/{accountId}/simulate-charge (para acionamento e testes da cena 7)
	mux.HandleFunc("POST /v1/budgets/{accountId}/simulate-charge", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		reqID := httputil.GetRequestID(r)

		var req SimulateChargeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Requisição Inválida",
				"Corpo JSON incorreto.")
			return
		}

		info, err := service.SimulateCharge(accountID, reqID, req.Amount)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
				"https://atlascloud.io/errors/internal-error",
				"Erro ao Injetar Consumo",
				err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	// POST /v1/budgets/{accountId}/reset (zera cobranças simuladas e recalcula faturamento)
	mux.HandleFunc("POST /v1/budgets/{accountId}/reset", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		info := service.ResetBudget(accountID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	// PUT /v1/budgets/{accountId}
	mux.HandleFunc("PUT /v1/budgets/{accountId}", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")

		var req UpdateBudgetLimitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BudgetLimit <= 0 {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Limite Inválido",
				"O teto de orçamento deve ser maior que zero.")
			return
		}

		info := service.UpdateLimit(accountID, req.BudgetLimit)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	handler := httputil.RequestIDMiddleware("atlas-billing-service", mux)

	serverAddr := ":" + port
	log.Printf("[atlas-billing-service] Inicializado e escutando na porta %s...", port)
	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[atlas-billing-service] Falha fatal no servidor HTTP: %v", err)
	}
}
