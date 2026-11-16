package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type DashboardAggregator struct {
	computeURL  string
	securityURL string
	billingURL  string
	client      *http.Client
}

func NewDashboardAggregator(computeURL, securityURL, billingURL string) *DashboardAggregator {
	return &DashboardAggregator{
		computeURL:  computeURL,
		securityURL: securityURL,
		billingURL:  billingURL,
		client:      &http.Client{Timeout: 2 * time.Second},
	}
}

type AggregatedDashboardResponse struct {
	AccountID        string                 `json:"account_id"`
	Status           string                 `json:"status"`  // HEALTHY ou DEGRADED
	Degraded         bool                   `json:"degraded"` // true quando ao menos 1 serviço falhou
	Parcial          bool                   `json:"parcial"`  // compatibilidade direta com o Tutorial de API Gateway Passo 6
	DegradedServices []string               `json:"degraded_services"`
	Compute          map[string]interface{} `json:"compute"`
	Security         map[string]interface{} `json:"security"`
	Billing          map[string]interface{} `json:"billing"`
	GeneratedAt      time.Time              `json:"generated_at"`
}

func (da *DashboardAggregator) GetDashboard(ctx context.Context, accountID, reqID string) AggregatedDashboardResponse {
	var wg sync.WaitGroup
	wg.Add(3)

	var (
		computeData  map[string]interface{}
		securityData map[string]interface{}
		billingData  map[string]interface{}
		degraded     []string
		mu           sync.Mutex
	)

	// 1. Consulta paralela ao Compute Service
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/v1/instances?account_id=%s&limit=50", da.computeURL, accountID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			mu.Lock()
			degraded = append(degraded, "compute")
			mu.Unlock()
			return
		}
		if reqID != "" {
			req.Header.Set("X-Request-Id", reqID)
		}

		resp, err := da.client.Do(req)
		if err != nil || resp.StatusCode >= 500 {
			mu.Lock()
			degraded = append(degraded, "compute")
			mu.Unlock()
			return
		}
		defer resp.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			mu.Lock()
			computeData = data
			mu.Unlock()
		} else {
			mu.Lock()
			degraded = append(degraded, "compute")
			mu.Unlock()
		}
	}()

	// 2. Consulta paralela ao Security Service
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/v1/security-groups?account_id=%s", da.securityURL, accountID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			mu.Lock()
			degraded = append(degraded, "security")
			mu.Unlock()
			return
		}
		if reqID != "" {
			req.Header.Set("X-Request-Id", reqID)
		}

		resp, err := da.client.Do(req)
		if err != nil || resp.StatusCode >= 500 {
			mu.Lock()
			degraded = append(degraded, "security")
			mu.Unlock()
			return
		}
		defer resp.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			mu.Lock()
			securityData = data
			mu.Unlock()
		} else {
			mu.Lock()
			degraded = append(degraded, "security")
			mu.Unlock()
		}
	}()

	// 3. Consulta paralela ao Billing Service
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/v1/budgets/%s", da.billingURL, accountID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			mu.Lock()
			degraded = append(degraded, "billing")
			mu.Unlock()
			return
		}
		if reqID != "" {
			req.Header.Set("X-Request-Id", reqID)
		}

		resp, err := da.client.Do(req)
		if err != nil || resp.StatusCode >= 500 {
			mu.Lock()
			degraded = append(degraded, "billing")
			mu.Unlock()
			return
		}
		defer resp.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			mu.Lock()
			billingData = data
			mu.Unlock()
		} else {
			mu.Lock()
			degraded = append(degraded, "billing")
			mu.Unlock()
		}
	}()

	wg.Wait()

	if degraded == nil {
		degraded = []string{}
	}

	isDegraded := len(degraded) > 0
	status := "HEALTHY"
	if isDegraded {
		status = "DEGRADED"
	}

	return AggregatedDashboardResponse{
		AccountID:        accountID,
		Status:           status,
		Degraded:         isDegraded,
		Parcial:          isDegraded,
		DegradedServices: degraded,
		Compute:          computeData,
		Security:         securityData,
		Billing:          billingData,
		GeneratedAt:      time.Now().UTC(),
	}
}
