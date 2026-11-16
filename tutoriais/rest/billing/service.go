package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"atlascloud/pkg/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

type BillingService struct {
	mu              sync.RWMutex
	budgets         map[string]*BudgetInfo
	simulatedCharge map[string]float64
	computeURL      string
	rabbitURL       string
	exchange        string
	conn            *amqp.Connection
	channel         *amqp.Channel
}

func NewBillingService(computeURL, rabbitURL, exchange string) *BillingService {
	s := &BillingService{
		budgets:         make(map[string]*BudgetInfo),
		simulatedCharge: make(map[string]float64),
		computeURL:      computeURL,
		rabbitURL:       rabbitURL,
		exchange:        exchange,
	}
	s.connectRabbitMQ()
	return s
}

func (s *BillingService) connectRabbitMQ() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rabbitURL == "" {
		return
	}

	conn, err := amqp.Dial(s.rabbitURL)
	if err != nil {
		log.Printf("[atlas-billing-service] [WARN] RabbitMQ offline (%v). Notificações serão omitidas.", err)
		return
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("[atlas-billing-service] [ERROR] Falha ao abrir canal AMQP: %v", err)
		_ = conn.Close()
		return
	}

	_ = ch.ExchangeDeclare(s.exchange, "topic", true, false, false, false, nil)

	s.conn = conn
	s.channel = ch
	log.Printf("[atlas-billing-service] Conectado ao RabbitMQ na exchange '%s'", s.exchange)
}

func (s *BillingService) publishEvent(ctx context.Context, routingKey, reqID string, payload interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.channel == nil || s.conn == nil || s.conn.IsClosed() {
		go s.connectRabbitMQ()
		return
	}

	event := events.NewEvent(routingKey, reqID, "atlas-billing", payload)
	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("[atlas-billing-service] Erro ao serializar evento %s: %v", routingKey, err)
		return
	}

	err = s.channel.PublishWithContext(
		ctx,
		s.exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			MessageId:    event.EventID,
			Timestamp:    event.Timestamp,
			Headers: amqp.Table{
				"X-Request-Id": reqID,
			},
			Body: body,
		},
	)
	if err != nil {
		log.Printf("[atlas-billing-service] [ERROR] Erro ao publicar evento %s: %v", routingKey, err)
	} else {
		log.Printf("[atlas-billing-service] [%s] EVENTO PUBLICADO: %s (EventID=%s)",
			reqID, routingKey, event.EventID)
	}
}

func (s *BillingService) publishBudgetExceeded(ctx context.Context, reqID string, info BudgetInfo) {
	s.publishEvent(ctx, events.TypeBudgetExceeded, reqID, info)
}

func (s *BillingService) GetBudget(ctx context.Context, accountID, reqID string) (*BudgetInfo, error) {
	s.mu.Lock()
	info, ok := s.budgets[accountID]
	if !ok {
		info = &BudgetInfo{
			AccountID:   accountID,
			BudgetLimit: 100.00, // Limite contratual padrão de $100.00
			Currency:    "USD",
		}
		s.budgets[accountID] = info
	}
	simCharge := s.simulatedCharge[accountID]
	s.mu.Unlock()

	// Buscar instâncias ativas do Compute Service
	breakdowns := []InstanceCostBreakdown{}
	var instanceCost float64 = 0.0

	if s.computeURL != "" {
		reqURL := fmt.Sprintf("%s/v1/instances?account_id=%s&status=RUNNING", s.computeURL, accountID)
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err == nil {
			if reqID != "" {
				req.Header.Set("X-Request-Id", reqID)
			}
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				var data struct {
					Items []struct {
						ID        string    `json:"id"`
						Name      string    `json:"name"`
						Flavor    string    `json:"flavor"`
						CreatedAt time.Time `json:"created_at"`
					} `json:"items"`
				}
				if json.NewDecoder(resp.Body).Decode(&data) == nil {
					now := time.Now().UTC()
					for _, item := range data.Items {
						hourlyRate := 0.02
						if item.Flavor == "c5.large" {
							hourlyRate = 0.10
						}
						// Calcular segundos de execução (mínimo de 30 segundos para efeito visual imediato)
						elapsed := now.Sub(item.CreatedAt).Seconds()
						if elapsed < 30 {
							elapsed = 30
						}
						cost := (elapsed / 3600.0) * hourlyRate
						instanceCost += cost

						breakdowns = append(breakdowns, InstanceCostBreakdown{
							InstanceID: item.ID,
							Name:       item.Name,
							Flavor:     item.Flavor,
							HourlyRate: hourlyRate,
							CostUSD:    cost,
						})
					}
				}
				_ = resp.Body.Close()
			}
		}
	}

	totalCurrentCost := instanceCost + simCharge
	exceeded := totalCurrentCost > info.BudgetLimit

	res := BudgetInfo{
		AccountID:      accountID,
		CurrentCost:    totalCurrentCost,
		BudgetLimit:    info.BudgetLimit,
		Currency:       "USD",
		Exceeded:       exceeded,
		ForecastedCost: totalCurrentCost * 1.25, // Projeção simples de fim de ciclo
		Instances:      breakdowns,
		LastUpdated:    time.Now().UTC(),
	}

	// Se excedeu e antes não estava marcado ou acabou de ser ultrapassado
	s.mu.Lock()
	wasExceeded := s.budgets[accountID].Exceeded
	s.budgets[accountID].CurrentCost = totalCurrentCost
	s.budgets[accountID].Exceeded = exceeded
	s.budgets[accountID].LastUpdated = res.LastUpdated
	s.mu.Unlock()

	if exceeded && !wasExceeded {
		go s.publishBudgetExceeded(context.Background(), reqID, res)
	}

	return &res, nil
}

func (s *BillingService) SimulateCharge(accountID, reqID string, amount float64) (*BudgetInfo, error) {
	s.mu.Lock()
	s.simulatedCharge[accountID] += amount
	s.mu.Unlock()

	info, err := s.GetBudget(context.Background(), accountID, reqID)
	if err == nil {
		// Evento rotineiro de tarifação -> Recebido apenas pelo Consumidor 2 (Auditoria: '#')
		go s.publishEvent(context.Background(), "billing.charge.recorded", reqID, *info)
		if info.Exceeded {
			// Evento de alerta crítico -> Recebido pelo Consumidor 1 (Notificações) e Consumidor 2 (Auditoria)
			go s.publishBudgetExceeded(context.Background(), reqID, *info)
		}
	}
	return info, err
}

func (s *BillingService) UpdateLimit(accountID string, newLimit float64) *BudgetInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, ok := s.budgets[accountID]
	if !ok {
		info = &BudgetInfo{AccountID: accountID, Currency: "USD"}
		s.budgets[accountID] = info
	}
	info.BudgetLimit = newLimit
	info.Exceeded = info.CurrentCost > newLimit
	info.LastUpdated = time.Now().UTC()
	return info
}

func (s *BillingService) ResetBudget(accountID string) *BudgetInfo {
	s.mu.Lock()
	s.simulatedCharge[accountID] = 0.0
	if info, ok := s.budgets[accountID]; ok {
		info.CurrentCost = 0.0
		info.Exceeded = false
		info.LastUpdated = time.Now().UTC()
	}
	s.mu.Unlock()

	info, _ := s.GetBudget(context.Background(), accountID, "")
	return info
}

