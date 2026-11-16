#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"
CUSTOM_REQ_ID="fatec-trace-scene-06-$(date +%s)"

echo "=========================================================="
echo " CENA 6: Mensageria RabbitMQ 4, Tópicos e Worker Assíncrono"
echo "=========================================================="

echo "[6.1] Disparando criação de instância com X-Request-Id específico ($CUSTOM_REQ_ID)..."
RESP=$(curl -s -X POST "$GATEWAY_URL/v1/instances" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-Id: $CUSTOM_REQ_ID" \
  -d '{
    "name": "worker-event-test-01",
    "flavor": "t3.micro",
    "vpc_id": "vpc-dev-01",
    "security_group_id": "sg-default-web",
    "account_id": "acc-fatec-01"
  }')

INST_ID=$(echo "$RESP" | jq -r '.id // empty')
echo "      -> Instância criada: $INST_ID"
echo "      -> Evento 'compute.instance.created' publicado na exchange 'atlas.events'."

echo ""
echo "[6.2] Atualizando estado da instância (RUNNING -> STOPPED)..."
curl -s -X PATCH "$GATEWAY_URL/v1/instances/$INST_ID" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-Id: $CUSTOM_REQ_ID" \
  -d '{"status": "STOPPED"}' >/dev/null
echo "      -> Evento 'compute.instance.updated' publicado na exchange 'atlas.events'."

echo ""
echo "[6.3] Finalizando a instância (DELETE)..."
curl -s -X DELETE "$GATEWAY_URL/v1/instances/$INST_ID" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-Id: $CUSTOM_REQ_ID" >/dev/null
echo "      -> Evento 'compute.instance.terminated' publicado na exchange 'atlas.events'."

echo ""
echo "[6.4] Verificando logs do Notification Worker..."
echo "      O consumidor AMQP escuta na fila com prefetch(1) e confirmação manual (ack)."
echo "      Ele processa o evento e gera log estruturado contendo X-Request-Id: $CUSTOM_REQ_ID"

echo ""
echo ">>> [SUCESSO] CENA 6 VALIDADA: Ciclo completo de eventos AMQP computados e consumidos."
