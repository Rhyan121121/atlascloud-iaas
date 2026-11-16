package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"atlascloud/pkg/httputil"

	"github.com/graphql-go/graphql"
)

type GraphQLRequestBody struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
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

	client := NewRestClient(computeURL, securityURL, billingURL)
	schema, err := BuildSchema(client)
	if err != nil {
		log.Fatalf("[atlas-graphql-service] Erro ao compilar schema GraphQL: %v", err)
	}

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"atlas-graphql-service"}`))
	})

	// Handler GraphQL (atende /graphql e / para compatibilidade com reescrita de caminho no Gateway)
	handleGraphQL := func(w http.ResponseWriter, r *http.Request) {
		var reqBody GraphQLRequestBody

		if r.Method == http.MethodGet {
			reqBody.Query = r.URL.Query().Get("query")
			reqBody.OperationName = r.URL.Query().Get("operationName")
		} else {
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Erro ao ler corpo da requisição", http.StatusBadRequest)
				return
			}
			_ = json.Unmarshal(bodyBytes, &reqBody)
		}

		if reqBody.Query == "" {
			http.Error(w, "Query GraphQL ausente", http.StatusBadRequest)
			return
		}

		reqID := httputil.GetRequestID(r)
		// Instancia um DataLoader NOVO por requisição para evitar vazamento de cache entre clientes (Seção 7.1 do Tutorial de GraphQL)
		loader := NewSecurityGroupDataLoader(client, reqID)
		ctx := context.WithValue(r.Context(), DataLoaderContextKey, loader)

		result := graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  reqBody.Query,
			VariableValues: reqBody.Variables,
			OperationName:  reqBody.OperationName,
			Context:        ctx,
		})

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", reqID)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}

	mux.HandleFunc("POST /graphql", handleGraphQL)
	mux.HandleFunc("GET /graphql", handleGraphQL)
	mux.HandleFunc("POST /", handleGraphQL)
	mux.HandleFunc("GET /", handleGraphQL)

	handler := httputil.RequestIDMiddleware("atlas-graphql-service", mux)

	serverAddr := ":" + port
	log.Printf("[atlas-graphql-service] Servidor GraphQL BFF ativo na porta %s...", port)
	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[atlas-graphql-service] Falha fatal no servidor: %v", err)
	}
}
