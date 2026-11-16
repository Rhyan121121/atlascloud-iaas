package events

import (
	"time"

	"github.com/google/uuid"
)

// Tipos de Eventos da Plataforma AtlasCloud
const (
	TypeInstanceCreated    = "compute.instance.created"
	TypeInstanceUpdated    = "compute.instance.updated"
	TypeInstanceTerminated = "compute.instance.terminated"
	TypeBudgetExceeded     = "billing.budget.exceeded"
)

// Event é a estrutura padronizada de eventos que trafegam pelo RabbitMQ
type Event struct {
	EventID   string      `json:"event_id"`
	EventType string      `json:"event_type"`
	RequestID string      `json:"request_id"`
	Timestamp time.Time   `json:"timestamp"`
	Producer  string      `json:"producer"`
	Payload   interface{} `json:"payload"`
}

// NewEvent constrói um novo evento gerando ID v7
func NewEvent(eventType, requestID, producer string, payload interface{}) Event {
	id, err := uuid.NewV7()
	idStr := ""
	if err == nil {
		idStr = id.String()
	} else {
		idStr = uuid.NewString()
	}

	return Event{
		EventID:   idStr,
		EventType: eventType,
		RequestID: requestID,
		Timestamp: time.Now().UTC(),
		Producer:  producer,
		Payload:   payload,
	}
}
