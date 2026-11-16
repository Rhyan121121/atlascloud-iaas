package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"atlascloud/pkg/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

type EventPublisher struct {
	url      string
	mu       sync.Mutex
	conn     *amqp.Connection
	channel  *amqp.Channel
	exchange string
}

func NewEventPublisher(url, exchange string) *EventPublisher {
	p := &EventPublisher{
		url:      url,
		exchange: exchange,
	}
	p.connect()
	return p
}

func (p *EventPublisher) connect() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.url == "" {
		return
	}

	conn, err := amqp.Dial(p.url)
	if err != nil {
		log.Printf("[atlas-compute-service] [WARN] RabbitMQ indisponível (%v). Mensagens serão omitidas no modo local.", err)
		return
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("[atlas-compute-service] [ERROR] Falha ao abrir canal AMQP: %v", err)
		_ = conn.Close()
		return
	}

	err = ch.ExchangeDeclare(
		p.exchange, // atlas.events
		"topic",    // tipo topic
		true,       // durable
		false,      // auto-deleted
		false,      // internal
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		log.Printf("[atlas-compute-service] [ERROR] Falha ao declarar exchange %s: %v", p.exchange, err)
		_ = ch.Close()
		_ = conn.Close()
		return
	}

	p.conn = conn
	p.channel = ch
	log.Printf("[atlas-compute-service] Conectado com sucesso ao RabbitMQ na exchange '%s'", p.exchange)
}

func (p *EventPublisher) Publish(ctx context.Context, routingKey string, event events.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.channel == nil || p.conn == nil || p.conn.IsClosed() {
		// Tentar reconectar uma vez
		go p.connect()
		log.Printf("[atlas-compute-service] [SKIP] Publicação de evento %s ignorada (RabbitMQ offline)", event.EventType)
		return
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("[atlas-compute-service] [ERROR] Erro ao serializar evento: %v", err)
		return
	}

	err = p.channel.PublishWithContext(
		ctx,
		p.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // Mensagem persistente
			ContentType:  "application/json",
			MessageId:    event.EventID,
			Timestamp:    event.Timestamp,
			Headers: amqp.Table{
				"X-Request-Id": event.RequestID,
			},
			Body: body,
		},
	)
	if err != nil {
		log.Printf("[atlas-compute-service] [ERROR] Falha ao publicar evento %s: %v", routingKey, err)
	} else {
		log.Printf("[atlas-compute-service] [%s] Evento publicado: routingKey='%s', eventID='%s'",
			event.RequestID, routingKey, event.EventID)
	}
}

func (p *EventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}
