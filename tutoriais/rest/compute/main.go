package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"atlascloud/pkg/events"
	"atlascloud/pkg/httputil"
	"atlascloud/pkg/rfc9457"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	repo := NewRepository()
	publisher := NewEventPublisher(rabbitURL, "atlas.events")
	defer publisher.Close()

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"atlas-compute-service"}`))
	})

	// POST /v1/instances
	mux.HandleFunc("POST /v1/instances", func(w http.ResponseWriter, r *http.Request) {
		reqID := httputil.GetRequestID(r)
		var req CreateInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Requisição Malformada",
				"O corpo da requisição não é um JSON válido.")
			return
		}

		var invalidParams []rfc9457.InvalidParam
		if strings.TrimSpace(req.Name) == "" {
			invalidParams = append(invalidParams, rfc9457.InvalidParam{Name: "name", Reason: "Obrigatório: nome da instância com ao menos 2 caracteres"})
		}
		if req.Flavor == "" {
			invalidParams = append(invalidParams, rfc9457.InvalidParam{Name: "flavor", Reason: "Obrigatório: deve ser t3.micro ou c5.large"})
		}
		if req.VPCID == "" {
			invalidParams = append(invalidParams, rfc9457.InvalidParam{Name: "vpc_id", Reason: "Obrigatório: identificador da rede VPC"})
		}
		if req.SecurityGroupID == "" {
			invalidParams = append(invalidParams, rfc9457.InvalidParam{Name: "security_group_id", Reason: "Obrigatório: identificador do Security Group"})
		}
		if len(invalidParams) > 0 {
			rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
				"https://atlascloud.io/errors/validation-error",
				"Parâmetros Obrigatórios Ausentes",
				"Os campos 'name', 'flavor', 'vpc_id' e 'security_group_id' são de preenchimento obrigatório.",
				invalidParams...,
			)
			return
		}

		inst, err := repo.Create(req)
		if err != nil {
			writeDomainError(w, r, err, req.Name, req.VPCID)
			return
		}

		// Publicar evento assíncrono no RabbitMQ
		event := events.NewEvent(events.TypeInstanceCreated, reqID, "atlas-compute", inst)
		go publisher.Publish(r.Context(), events.TypeInstanceCreated, event)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", fmt.Sprintf("/v1/instances/%s", inst.ID))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(inst)
	})

	// GET /v1/instances (lista paginada com filtros, ordenação e teto de 50 itens)
	mux.HandleFunc("GET /v1/instances", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Suporte a atraso artificial (?atraso=3000 ou ?delay=3000) para teste de Timeout 504 no Gateway (Restrição EARS [R5])
		delayStr := q.Get("atraso")
		if delayStr == "" {
			delayStr = q.Get("delay")
		}
		if delayStr != "" {
			if d, err := strconv.Atoi(delayStr); err == nil && d > 0 {
				time.Sleep(time.Duration(d) * time.Millisecond)
			}
		}

		status := q.Get("status")
		if status != "" {
			stUpper := strings.ToUpper(status)
			if stUpper != string(StatusProvisioning) && stUpper != string(StatusRunning) && stUpper != string(StatusStopped) && stUpper != string(StatusTerminated) {
				rfc9457.WriteProblem(w, r, http.StatusBadRequest,
					"https://atlascloud.io/errors/invalid-parameter",
					"Parâmetro de Filtro Inválido",
					"O parâmetro 'status' deve ser um de: PROVISIONING, RUNNING, STOPPED, TERMINATED.")
				return
			}
		}

		flavor := q.Get("flavor")
		if flavor != "" {
			if _, ok := FlavorSpecs[flavor]; !ok {
				rfc9457.WriteProblem(w, r, http.StatusBadRequest,
					"https://atlascloud.io/errors/invalid-parameter",
					"Parâmetro de Filtro Inválido",
					"O parâmetro 'flavor' deve ser um de: t3.micro, c5.large.")
				return
			}
		}

		vpcID := q.Get("vpc_id")
		sgID := q.Get("security_group_id")
		accountID := q.Get("account_id")
		search := q.Get("search")
		if search == "" {
			search = q.Get("busca")
		}

		page := 1
		pageRaw := q.Get("page")
		if pageRaw == "" {
			pageRaw = q.Get("pagina")
		}
		if pageRaw != "" {
			p, err := strconv.Atoi(pageRaw)
			if err != nil || p < 1 {
				rfc9457.WriteProblem(w, r, http.StatusBadRequest,
					"https://atlascloud.io/errors/invalid-parameter",
					"Parâmetro de Paginação Inválido",
					"O parâmetro 'page'/'pagina' deve ser um número inteiro positivo (>= 1).")
				return
			}
			page = p
		}

		limit := 10
		limitRaw := q.Get("limit")
		if limitRaw == "" {
			limitRaw = q.Get("limite")
		}
		if limitRaw != "" {
			l, err := strconv.Atoi(limitRaw)
			if err != nil || l < 1 {
				rfc9457.WriteProblem(w, r, http.StatusBadRequest,
					"https://atlascloud.io/errors/invalid-parameter",
					"Parâmetro de Paginação Inválido",
					"O parâmetro 'limit'/'limite' deve ser um número inteiro positivo (>= 1).")
				return
			}
			limit = l
		}

		sortBy := q.Get("sort_by")
		if sortBy == "" {
			sortBy = q.Get("ordenar")
		}
		if sortBy == "" {
			sortBy = "created_at"
		}
		order := q.Get("order")
		if order == "" {
			order = q.Get("ordem")
		}
		if order == "" {
			order = "desc"
		}
		if !strings.EqualFold(order, "asc") && !strings.EqualFold(order, "desc") {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/invalid-parameter",
				"Parâmetro de Ordenação Inválido",
				"O parâmetro 'order'/'ordem' deve ser 'asc' ou 'desc'.")
			return
		}

		res := repo.List(status, flavor, vpcID, sgID, accountID, search, page, limit, sortBy, order)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	})

	// GET /v1/instances/{id}
	mux.HandleFunc("GET /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		inst, err := repo.GetByID(id)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusNotFound,
				"https://atlascloud.io/errors/not-found",
				"Instância Não Encontrada",
				fmt.Sprintf("Nenhuma instância encontrada com o identificador '%s'.", id))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(inst)
	})

	// PUT /v1/instances/{id} (Substituição completa do recurso)
	mux.HandleFunc("PUT /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		reqID := httputil.GetRequestID(r)

		var req CreateInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Corpo JSON Inválido",
				"Erro ao decodificar a carga JSON da requisição.")
			return
		}

		if strings.TrimSpace(req.Name) == "" || req.Flavor == "" || req.VPCID == "" || req.SecurityGroupID == "" {
			rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
				"https://atlascloud.io/errors/validation-error",
				"Parâmetros Obrigatórios Ausentes no PUT",
				"O método PUT exige a representação completa: 'name', 'flavor', 'vpc_id' e 'security_group_id'.")
			return
		}

		inst, err := repo.Replace(id, req)
		if err != nil {
			writeDomainError(w, r, err, req.Name, req.VPCID)
			return
		}

		event := events.NewEvent(events.TypeInstanceUpdated, reqID, "atlas-compute", inst)
		go publisher.Publish(r.Context(), events.TypeInstanceUpdated, event)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(inst)
	})

	// PATCH /v1/instances/{id} (Alteração parcial segura contra mass assignment)
	mux.HandleFunc("PATCH /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		reqID := httputil.GetRequestID(r)

		var req UpdateInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusBadRequest,
				"https://atlascloud.io/errors/bad-request",
				"Corpo JSON Inválido",
				"Erro ao decodificar a carga JSON da requisição.")
			return
		}

		inst, err := repo.Update(id, req)
		if err != nil {
			writeDomainError(w, r, err, "", "")
			return
		}

		eventType := events.TypeInstanceUpdated
		if inst.Status == StatusTerminated {
			eventType = events.TypeInstanceTerminated
		}
		event := events.NewEvent(eventType, reqID, "atlas-compute", inst)
		go publisher.Publish(r.Context(), eventType, event)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(inst)
	})

	// POST /v1/instances/purge
	mux.HandleFunc("POST /v1/instances/purge", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("account_id")
		count := repo.PurgeTerminated(accountID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"purged_count": count,
			"status":       "SUCCESS",
		})
	})

	// DELETE /v1/instances/{id}
	mux.HandleFunc("DELETE /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		reqID := httputil.GetRequestID(r)
		purge := r.URL.Query().Get("purge") == "true"

		inst, err := repo.GetByID(id)
		if err != nil {
			rfc9457.WriteProblem(w, r, http.StatusNotFound,
				"https://atlascloud.io/errors/not-found",
				"Instância Não Encontrada",
				fmt.Sprintf("Instância '%s' não localizada.", id))
			return
		}

		if purge || inst.Status == StatusTerminated {
			_ = repo.PurgeByID(id)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err := repo.Delete(id); err != nil {
			rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
				"https://atlascloud.io/errors/internal-error",
				"Falha ao Excluir Instância",
				err.Error())
			return
		}

		event := events.NewEvent(events.TypeInstanceTerminated, reqID, "atlas-compute", inst)
		go publisher.Publish(r.Context(), events.TypeInstanceTerminated, event)

		w.WriteHeader(http.StatusNoContent)
	})

	handler := httputil.RequestIDMiddleware("atlas-compute-service", mux)

	serverAddr := ":" + port
	log.Printf("[atlas-compute-service] Inicializado e escutando na porta %s...", port)
	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[atlas-compute-service] Falha fatal no servidor HTTP: %v", err)
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error, name, vpcID string) {
	switch {
	case errors.Is(err, ErrNotFound):
		rfc9457.WriteProblem(w, r, http.StatusNotFound,
			"https://atlascloud.io/errors/not-found",
			"Instância Não Encontrada",
			"Nenhuma instância localizada com o identificador informado.")
	case errors.Is(err, ErrQuotaExceeded):
		rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
			"https://atlascloud.io/errors/quota-exceeded",
			"Cota de vCPUs Excedida",
			fmt.Sprintf("A operação violaria a cota máxima permitida de %d vCPUs ativas para esta conta (Restrição EARS [R7]).", AccountVCPUQuota))
	case errors.Is(err, ErrDuplicateActiveName):
		detail := "Outra instância ativa nesta VPC já utiliza este nome."
		if name != "" && vpcID != "" {
			detail = fmt.Sprintf("Já existe uma instância ativa com o nome '%s' na VPC '%s'.", name, vpcID)
		}
		rfc9457.WriteProblem(w, r, http.StatusConflict,
			"https://atlascloud.io/errors/conflict",
			"Nome de Instância Conflitante",
			detail)
	case errors.Is(err, ErrFlavorRunning):
		rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
			"https://atlascloud.io/errors/invalid-state-transition",
			"Transição de Estado Inválida",
			"Não é permitido alterar o flavor de uma instância no estado RUNNING. Pare a instância (STOPPED) antes de redimensionar.")
	case errors.Is(err, ErrInvalidFlavor):
		rfc9457.WriteProblem(w, r, http.StatusUnprocessableEntity,
			"https://atlascloud.io/errors/validation-error",
			"Flavor Inválido",
			"Os flavors permitidos são exclusivamente 't3.micro' e 'c5.large'.")
	default:
		rfc9457.WriteProblem(w, r, http.StatusInternalServerError,
			"https://atlascloud.io/errors/internal-error",
			"Erro Interno",
			err.Error())
	}
}
