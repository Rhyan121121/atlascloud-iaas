package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"atlascloud/pkg/httputil"
	"atlascloud/pkg/rfc9457"
)

func checkActiveInstancesUsingSG(computeURL, sgID, reqID string) (bool, error) {
	if computeURL == "" {
		return false, nil
	}

	url := fmt.Sprintf("%s/v1/instances?security_group_id=%s", computeURL, sgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Se compute estiver offline, loga e permite para não travar ambiente isolado
		log.Printf("[atlas-security-service] [WARN] Falha ao consultar compute service: %v", err)
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var data struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, err
	}

	for _, inst := range data.Items {
		if inst.Status != "TERMINATED" {
			return true, nil
		}
	}
	return false, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	computeURL := os.Getenv("COMPUTE_SERVICE_URL")
	if computeURL == "" {
		computeURL = "http://127.0.0.1:8081"
	}

	repo := NewRepository()

	// Seed básico para testes imediatos
	_, _ = repo.Create(CreateSecurityGroupRequest{
		Name:      "sg-default-web",
		VPCID:     "vpc-dev-01",
		AccountID: "acc-fatec-01",
		Rules: []CreateRuleRequest{
			{Protocol: "TCP", StartPort: 80, EndPort: 80, CIDROrigin: "0.0.0.0/0"},
			{Protocol: "TCP", StartPort: 443, EndPort: 443, CIDROrigin: "0.0.0.0/0"},
		},
	})
	_, _ = repo.Create(CreateSecurityGroupRequest{
		Name:      "sg-default-db",
		VPCID:     "vpc-dev-01",
		AccountID: "acc-fatec-01",
		Rules: []CreateRuleRequest{
			{Protocol: "TCP", StartPort: 5432, EndPort: 5432, CIDROrigin: "10.0.0.0/16"},
		},
	})

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"atlas-security-service"}`))
	})

	// POST /v1/security-groups
	mux.HandleFunc("POST /v1/security-groups", func(w http.ResponseWriter, r *http.Request) {
		var req CreateSecurityGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Requisição Inválida",
				"Formato JSON incorreto.")
			return
		}

		sg, err := repo.Create(req)
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidPort):
				rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
					"https://atlascloud.io/errors/invalid-port-range",
					"Intervalo de Portas Inválido",
					"As portas devem ser números inteiros entre 1 e 65535, e start_port deve ser menor ou igual a end_port.",
					rfc9457.InvalidParam{Name: "start_port", Reason: "Deve ser entre 1 e 65535"},
					rfc9457.InvalidParam{Name: "end_port", Reason: "Deve ser entre 1 e 65535 e >= start_port"},
				)
			case errors.Is(err, ErrInvalidProto):
				rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
					"https://atlascloud.io/errors/validation-error",
					"Protocolo Não Suportado",
					"Os protocolos suportados são TCP, UDP e ICMP.")
			case errors.Is(err, ErrMissingFields):
				rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
					"https://atlascloud.io/errors/validation-error",
					"Campos Obrigatórios Ausentes",
					"Os campos 'name' e 'vpc_id' são obrigatórios.")
			default:
				rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
					"https://atlascloud.io/errors/internal-error",
					"Erro Interno",
					err.Error())
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", fmt.Sprintf("/v1/security-groups/%s", sg.ID))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sg)
	})

	// GET /v1/security-groups
	mux.HandleFunc("GET /v1/security-groups", func(w http.ResponseWriter, r *http.Request) {
		vpcID := r.URL.Query().Get("vpc_id")
		accountID := r.URL.Query().Get("account_id")

		items := repo.List(vpcID, accountID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": items,
			"total": len(items),
		})
	})

	// GET /v1/security-groups/{id}
	mux.HandleFunc("GET /v1/security-groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sg, err := repo.GetByID(id)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusNotFound,
				"https://atlascloud.io/errors/not-found",
				"Security Group Não Encontrado",
				fmt.Sprintf("Nenhum Security Group encontrado com ID '%s'.", id))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sg)
	})

	// DELETE /v1/security-groups/{id}
	mux.HandleFunc("DELETE /v1/security-groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		reqID := httputil.GetRequestID(r)

		// 1. Verificar se existe
		_, err := repo.GetByID(id)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusNotFound,
				"https://atlascloud.io/errors/not-found",
				"Security Group Não Encontrado",
				fmt.Sprintf("Security Group '%s' não existe.", id))
			return
		}

		// 2. Verificar se está vinculado a instâncias ativas (409 Conflict)
		inUse, err := checkActiveInstancesUsingSG(computeURL, id, reqID)
		if err != nil {
			log.Printf("[atlas-security-service] Erro ao validar instâncias ativas: %v", err)
		}
		if inUse {
			rfc9457.WriteProblem(w, r, http.StatusConflict,
				"https://atlascloud.io/errors/dependency-conflict",
				"Recurso em Uso",
				fmt.Sprintf("O Security Group '%s' está vinculado a instâncias ativas no Compute Service e não pode ser excluído.", id))
			return
		}

		// 3. Excluir
		if err := repo.Delete(id); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
				"https://atlascloud.io/errors/internal-error",
				"Falha na Exclusão",
				err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	handler := httputil.RequestIDMiddleware("atlas-security-service", mux)

	serverAddr := ":" + port
	log.Printf("[atlas-security-service] Inicializado e escutando na porta %s...", port)
	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[atlas-security-service] Falha fatal no servidor HTTP: %v", err)
	}
}
