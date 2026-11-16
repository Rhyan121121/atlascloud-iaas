package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type RestClient struct {
	computeURL  string
	securityURL string
	billingURL  string
	httpClient  *http.Client
}

func NewRestClient(computeURL, securityURL, billingURL string) *RestClient {
	return &RestClient{
		computeURL:  computeURL,
		securityURL: securityURL,
		billingURL:  billingURL,
		httpClient:  &http.Client{Timeout: 3 * time.Second},
	}
}

type InstanceDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Flavor          string `json:"flavor"`
	Status          string `json:"status"`
	IPPrivado       string `json:"ip_privado"`
	IPPublico       string `json:"ip_publico"`
	VPCID           string `json:"vpc_id"`
	SecurityGroupID string `json:"security_group_id"`
	AccountID       string `json:"account_id"`
	VCPUs           int    `json:"vcpus"`
	MemoryGB        int    `json:"memory_gb"`
	CreatedAt       string `json:"created_at"`
}

type FirewallRuleDTO struct {
	ID         string `json:"id"`
	Protocol   string `json:"protocol"`
	StartPort  int    `json:"start_port"`
	EndPort    int    `json:"end_port"`
	CIDROrigin string `json:"cidr_origin"`
}

type SecurityGroupDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	VPCID     string            `json:"vpc_id"`
	AccountID string            `json:"account_id"`
	Rules     []FirewallRuleDTO `json:"rules"`
}

type BudgetDTO struct {
	AccountID      string  `json:"account_id"`
	CurrentCost    float64 `json:"current_cost"`
	BudgetLimit    float64 `json:"budget_limit"`
	Currency       string  `json:"currency"`
	Exceeded       bool    `json:"exceeded"`
	ForecastedCost float64 `json:"forecasted_cost"`
}

type ProblemDTO struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// SecurityGroupDataLoader implementa Batching + Cache por requisição para resolver o problema N+1 (Ponto Extra)
type SecurityGroupDataLoader struct {
	mu      sync.Mutex
	loaded  bool
	byID    map[string]SecurityGroupDTO
	client  *RestClient
	reqID   string
}

func NewSecurityGroupDataLoader(client *RestClient, reqID string) *SecurityGroupDataLoader {
	return &SecurityGroupDataLoader{
		byID:   make(map[string]SecurityGroupDTO),
		client: client,
		reqID:  reqID,
	}
}

func (l *SecurityGroupDataLoader) Load(ctx context.Context, sgID string) (*SecurityGroupDTO, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.loaded {
		groups, err := l.client.FetchSecurityGroups(ctx, "", l.reqID)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			l.byID[g.ID] = g
		}
		l.loaded = true
	}

	if g, ok := l.byID[sgID]; ok {
		cp := g
		return &cp, nil
	}
	return nil, nil
}

func (l *SecurityGroupDataLoader) PrimeAll(groups []SecurityGroupDTO) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, g := range groups {
		l.byID[g.ID] = g
	}
	l.loaded = true
}

func (c *RestClient) FetchInstancesFiltered(ctx context.Context, accountID, status, flavor, reqID string) ([]InstanceDTO, error) {
	q := url.Values{}
	q.Set("limit", "50")
	if accountID != "" {
		q.Set("account_id", accountID)
	}
	if status != "" {
		q.Set("status", status)
	}
	if flavor != "" {
		q.Set("flavor", flavor)
	}

	reqURL := fmt.Sprintf("%s/v1/instances?%s", c.computeURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("compute service returned status %d", resp.StatusCode)
	}

	var data struct {
		Items []InstanceDTO `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func (c *RestClient) FetchInstances(ctx context.Context, accountID, reqID string) ([]InstanceDTO, error) {
	return c.FetchInstancesFiltered(ctx, accountID, "", "", reqID)
}

func (c *RestClient) FetchInstanceByID(ctx context.Context, id, reqID string) (*InstanceDTO, error) {
	reqURL := fmt.Sprintf("%s/v1/instances/%s", c.computeURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var inst InstanceDTO
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func (c *RestClient) CreateInstance(ctx context.Context, payload map[string]interface{}, reqID string) (*InstanceDTO, int, string, error) {
	bodyBytes, _ := json.Marshal(payload)
	reqURL := fmt.Sprintf("%s/v1/instances", c.computeURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 500, "Erro ao criar requisição HTTP", err
	}
	req.Header.Set("Content-Type", "application/json")
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 502, "Serviço de Compute indisponível", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var prob ProblemDTO
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		msg := prob.Detail
		if msg == "" {
			msg = prob.Title
		}
		if msg == "" {
			msg = fmt.Sprintf("Erro HTTP %d na API REST", resp.StatusCode)
		}
		return nil, resp.StatusCode, msg, fmt.Errorf("%s", msg)
	}

	var inst InstanceDTO
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return nil, 500, "Erro ao decodificar resposta", err
	}
	return &inst, resp.StatusCode, "", nil
}

func (c *RestClient) UpdateInstanceStatus(ctx context.Context, id, status, reqID string) (*InstanceDTO, int, string, error) {
	bodyBytes, _ := json.Marshal(map[string]string{"status": status})
	reqURL := fmt.Sprintf("%s/v1/instances/%s", c.computeURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, "PATCH", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 500, "Erro ao criar requisição HTTP", err
	}
	req.Header.Set("Content-Type", "application/json")
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 502, "Serviço de Compute indisponível", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var prob ProblemDTO
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		msg := prob.Detail
		if msg == "" {
			msg = prob.Title
		}
		return nil, resp.StatusCode, msg, fmt.Errorf("%s", msg)
	}

	var inst InstanceDTO
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return nil, 500, "Erro ao decodificar resposta", err
	}
	return &inst, resp.StatusCode, "", nil
}

func (c *RestClient) FetchSecurityGroups(ctx context.Context, accountID, reqID string) ([]SecurityGroupDTO, error) {
	reqURL := fmt.Sprintf("%s/v1/security-groups", c.securityURL)
	if accountID != "" {
		reqURL = fmt.Sprintf("%s?account_id=%s", reqURL, url.QueryEscape(accountID))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("security service returned status %d", resp.StatusCode)
	}

	var data struct {
		Items []SecurityGroupDTO `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func (c *RestClient) FetchBudget(ctx context.Context, accountID, reqID string) (*BudgetDTO, error) {
	reqURL := fmt.Sprintf("%s/v1/budgets/%s", c.billingURL, url.PathEscape(accountID))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("billing service returned status %d", resp.StatusCode)
	}

	var budget BudgetDTO
	if err := json.NewDecoder(resp.Body).Decode(&budget); err != nil {
		return nil, err
	}
	return &budget, nil
}
