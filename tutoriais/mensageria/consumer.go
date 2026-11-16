package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"atlascloud/pkg/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

const MaxRetries = 3

type Worker struct {
	rabbitURL            string
	notifIdempotency     *IdempotencyStore
	auditIdempotency     *IdempotencyStore
}

func NewWorker(rabbitURL string) *Worker {
	return &Worker{
		rabbitURL:        rabbitURL,
		notifIdempotency: NewIdempotencyStore(),
		auditIdempotency: NewIdempotencyStore(),
	}
}

func (w *Worker) Start() {
	for {
		log.Printf("[atlas-notification-worker] Conectando ao RabbitMQ em %s...", w.rabbitURL)
		conn, err := amqp.Dial(w.rabbitURL)
		if err != nil {
			log.Printf("[atlas-notification-worker] Falha ao conectar (%v). Nova tentativa em 3s...", err)
			time.Sleep(3 * time.Second)
			continue
		}

		ch, err := conn.Channel()
		if err != nil {
			log.Printf("[atlas-notification-worker] Erro ao abrir canal: %v", err)
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		// 1. Declarar Dead Letter Exchange (DLX) e Dead Letter Queue (DLQ)
		dlxName := "atlas.dlx"
		dlqName := "atlas.dlq"

		if err := ch.ExchangeDeclare(dlxName, "direct", true, false, false, false, nil); err != nil {
			log.Printf("[atlas-notification-worker] Erro ao declarar DLX: %v", err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		if _, err := ch.QueueDeclare(dlqName, true, false, false, false, nil); err != nil {
			log.Printf("[atlas-notification-worker] Erro ao declarar DLQ: %v", err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		if err := ch.QueueBind(dlqName, dlqName, dlxName, false, nil); err != nil {
			log.Printf("[atlas-notification-worker] Erro ao vincular DLQ à DLX: %v", err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		// 2. Declarar Exchange principal (tipo topic, durable)
		mainExchange := "atlas.events"
		if err := ch.ExchangeDeclare(mainExchange, "topic", true, false, false, false, nil); err != nil {
			log.Printf("[atlas-notification-worker] Erro ao declarar exchange %s: %v", mainExchange, err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		// 3. Declarar Fila do Consumidor 1: Notificações (recebe apenas parte dos eventos: billing.budget.exceeded e compute.instance.*)
		notifQueueName := "atlas.notifications.queue"
		dlqArgs := amqp.Table{
			"x-dead-letter-exchange":    dlxName,
			"x-dead-letter-routing-key": dlqName,
		}

		notifQueue, err := ch.QueueDeclare(notifQueueName, true, false, false, false, dlqArgs)
		if err != nil {
			log.Printf("[atlas-notification-worker] Erro ao declarar fila %s: %v", notifQueueName, err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		notifRoutingKeys := []string{"billing.budget.exceeded", "compute.instance.*"}
		for _, rk := range notifRoutingKeys {
			if err := ch.QueueBind(notifQueue.Name, rk, mainExchange, false, nil); err != nil {
				log.Printf("[atlas-notification-worker] Erro ao vincular chave %s na fila %s: %v", rk, notifQueue.Name, err)
			}
		}

		// 4. Declarar Fila do Consumidor 2: Auditoria e Conformidade (recebe TODOS os eventos com padrão '#')
		auditQueueName := "atlas.audit.queue"
		auditQueue, err := ch.QueueDeclare(auditQueueName, true, false, false, false, dlqArgs)
		if err != nil {
			log.Printf("[atlas-audit-worker] Erro ao declarar fila %s: %v", auditQueueName, err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		if err := ch.QueueBind(auditQueue.Name, "#", mainExchange, false, nil); err != nil {
			log.Printf("[atlas-audit-worker] Erro ao vincular chave '#' na fila %s: %v", auditQueue.Name, err)
		}

		// 5. Prefetch(1) para distribuição justa
		if err := ch.Qos(1, 0, false); err != nil {
			log.Printf("[atlas-notification-worker] Erro ao definir Prefetch(1): %v", err)
		}

		notifMsgs, err := ch.Consume(
			notifQueue.Name,
			"atlas-notification-consumer",
			false, // Confirmação manual (manual ACK/NACK)
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			log.Printf("[atlas-notification-worker] Erro ao iniciar consumo de notificações: %v", err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		auditMsgs, err := ch.Consume(
			auditQueue.Name,
			"atlas-audit-consumer",
			false, // Confirmação manual (manual ACK/NACK)
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			log.Printf("[atlas-audit-worker] Erro ao iniciar consumo de auditoria: %v", err)
			_ = ch.Close()
			_ = conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[Consumidor 1: Notificações] Ativo na fila '%s' (bindings: %v) com Prefetch(1) e DLQ '%s'",
			notifQueueName, notifRoutingKeys, dlqName)
		log.Printf("[Consumidor 2: Auditoria]    Ativo na fila '%s' (binding: '#') com Prefetch(1) e DLQ '%s'",
			auditQueueName, dlqName)

		done := make(chan struct{})
		go func() {
			for d := range notifMsgs {
				w.handleNotificationDelivery(ch, notifQueueName, d)
			}
			close(done)
		}()

		go func() {
			for d := range auditMsgs {
				w.handleAuditDelivery(d)
			}
		}()

		<-done
		log.Printf("[atlas-notification-worker] Conexão com RabbitMQ encerrada. Tentando reconectar...")
		_ = ch.Close()
		_ = conn.Close()
		time.Sleep(2 * time.Second)
	}
}

func extractRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	v, ok := headers["x-tentativas"]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func shouldSimulateFailure(body []byte) bool {
	s := string(body)
	return strings.Contains(s, `"simulate_failure":true`) ||
		strings.Contains(s, `"simulate_failure": true`) ||
		strings.Contains(s, `"falhar":true`) ||
		strings.Contains(s, `"falhar": true`)
}

// handleNotificationDelivery processa mensagens do Consumidor 1 (Notificações por E-mail + Retry Limitado + DLQ + Idempotência)
func (w *Worker) handleNotificationDelivery(ch *amqp.Channel, queueName string, d amqp.Delivery) {
	// 1. Tentar desserializar evento JSON
	var event events.Event
	if err := json.Unmarshal(d.Body, &event); err != nil {
		// Mensagem malformada (Poison Message) -> Rejeita SEM requeue para cair direto na DLQ!
		log.Printf("[Consumidor 1: Notificações] [DLQ_REJECT] Mensagem malformada rejeitada para DLQ (RoutingKey='%s', Erro=%v): %s",
			d.RoutingKey, err, string(d.Body))
		_ = d.Nack(false, false)
		return
	}

	reqID := event.RequestID
	if reqID == "" && d.Headers != nil {
		if val, ok := d.Headers["X-Request-Id"].(string); ok {
			reqID = val
		}
	}

	// 2. Simulação de falha transitória com Nova Tentativa Limitada (Passo 5.4 do Tutorial de Mensageria)
	if shouldSimulateFailure(d.Body) {
		tentativas := extractRetryCount(d.Headers) + 1
		if tentativas < MaxRetries {
			newHeaders := amqp.Table{
				"X-Request-Id": reqID,
				"x-tentativas": tentativas,
			}
			_ = ch.PublishWithContext(
				context.Background(),
				"",        // exchange padrão (publica direto na fila para retentativa)
				queueName, // atlas.notifications.queue
				false,
				false,
				amqp.Publishing{
					DeliveryMode: amqp.Persistent,
					ContentType:  "application/json",
					MessageId:    event.EventID,
					Timestamp:    time.Now().UTC(),
					Headers:      newHeaders,
					Body:         d.Body,
				},
			)
			_ = d.Ack(false)
			log.Printf("[Consumidor 1: Notificações] [%s] [RETRY] Falha no envio (tentativa %d/%d) para EventID='%s' -> reenfileirada",
				reqID, tentativas, MaxRetries, event.EventID)
			return
		}

		// Atingiu o limite de 3 tentativas -> Desiste e envia para a DLQ!
		log.Printf("[Consumidor 1: Notificações] [%s] [DLQ_REJECT] EventID='%s' falhou %d vezes seguidas -> desviada para DLQ (atlas.dlq)",
			reqID, event.EventID, tentativas)
		_ = d.Nack(false, false)
		return
	}

	// 3. Teste de Idempotência: Se o EventID já foi processado, descarta duplicata sem repetir efeito
	if !w.notifIdempotency.MarkIfNew(event.EventID) {
		log.Printf("[Consumidor 1: Notificações] [%s] [IDEMPOTENCY_SKIP] Evento duplicado ignorado com sucesso: EventID='%s', RoutingKey='%s'",
			reqID, event.EventID, d.RoutingKey)
		_ = d.Ack(false)
		return
	}

	// 4. Processamento do evento e simulação de envio de e-mail
	var subject, bodyPreview string
	recipient := "ops-team@atlascloud.io"

	switch event.EventType {
	case events.TypeBudgetExceeded:
		recipient = "finops-alerts@atlascloud.io"
		subject = "[ALERTA CRÍTICO] Teto de Orçamento Excedido na Conta"
		bodyPreview = fmt.Sprintf("A fatura ultrapassou o limite contratual de $ 100.00. Payload: %v", event.Payload)

	case events.TypeInstanceCreated:
		subject = "[COMPUTE] Nova Instância Virtual Provisionada"
		bodyPreview = fmt.Sprintf("Instância criada com sucesso. Dados: %v", event.Payload)

	case events.TypeInstanceUpdated:
		subject = "[COMPUTE] Estado de Instância Virtual Atualizado"
		bodyPreview = fmt.Sprintf("Instância modificada. Dados: %v", event.Payload)

	case events.TypeInstanceTerminated:
		subject = "[COMPUTE] Instância Virtual Finalizada/Deletada"
		bodyPreview = fmt.Sprintf("Recurso liberado. Dados: %v", event.Payload)

	default:
		subject = fmt.Sprintf("[EVENTO] Notificação AtlasCloud: %s", event.EventType)
		bodyPreview = fmt.Sprintf("Dados do evento: %v", event.Payload)
	}

	log.Printf("[Consumidor 1: Notificações] [EMAIL_SENT] Destinatário: %s | X-Request-Id: %s | EventID: %s | Assunto: %s | Detalhes: %s",
		recipient, reqID, event.EventID, subject, bodyPreview)

	_ = d.Ack(false)
}

// handleAuditDelivery processa mensagens do Consumidor 2 (Trilha de Auditoria e Compliance — recebe '#' de toda a exchange)
func (w *Worker) handleAuditDelivery(d amqp.Delivery) {
	var event events.Event
	if err := json.Unmarshal(d.Body, &event); err != nil {
		log.Printf("[Consumidor 2: Auditoria] [AUDIT_MALFORMED_ACK] Ignorando payload não-JSON na auditoria (já encaminhado à DLQ pelo canal principal)")
		_ = d.Ack(false)
		return
	}

	reqID := event.RequestID
	if reqID == "" && d.Headers != nil {
		if val, ok := d.Headers["X-Request-Id"].(string); ok {
			reqID = val
		}
	}

	if !w.auditIdempotency.MarkIfNew(event.EventID) {
		log.Printf("[Consumidor 2: Auditoria] [%s] [IDEMPOTENCY_SKIP] Registro de auditoria duplicado ignorado: EventID='%s'",
			reqID, event.EventID)
		_ = d.Ack(false)
		return
	}

	log.Printf("[Consumidor 2: Auditoria] [AUDIT_LOG] X-Request-Id: %s | EventID: %s | RoutingKey: %s | Origem: %s | TimestampUTC: %s",
		reqID, event.EventID, d.RoutingKey, event.Producer, event.Timestamp.UTC().Format(time.RFC3339))

	_ = d.Ack(false)
}
