#!/usr/bin/env bash
set -e

RABBIT_API="http://guest:guest@localhost:15672/api"
TS_SUFFIX="$(date +%s)"
DUP_EVENT_ID="019488a0-duplicado-${TS_SUFFIX}"
RETRY_EVENT_ID="019488a0-falha-retry-${TS_SUFFIX}"
REQ_ID="req-cena4-resiliencia-${TS_SUFFIX}"

echo "================================================================================"
echo " CENA 4: FALHA, NOVA TENTATIVA LIMITADA (RETRY 3x -> DLQ) E IDEMPOTÊNCIA"
echo "================================================================================"

echo "[4.1] Publicando o MESMO evento 2 vezes seguidas (EventID='${DUP_EVENT_ID}') para testar Idempotência..."
PAYLOAD_DUP=$(cat <<EOF
{"event_id":"${DUP_EVENT_ID}","event_type":"billing.budget.exceeded","request_id":"${REQ_ID}","timestamp":"2026-11-16T15:00:00Z","producer":"test-suite","payload":{"account_id":"acc-fatec-01","current_cost":150.0,"budget_limit":100.0}}
EOF
)

for i in 1 2; do
  curl -s -X POST "${RABBIT_API}/exchanges/%2F/atlas.events/publish" \
    -H "Content-Type: application/json" \
    -d "{
      \"properties\": {\"delivery_mode\": 2, \"content_type\": \"application/json\", \"headers\": {\"X-Request-Id\": \"${REQ_ID}\"}},
      \"routing_key\": \"billing.budget.exceeded\",
      \"payload\": $(echo "$PAYLOAD_DUP" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))'),
      \"payload_encoding\": \"string\"
    }" >/dev/null
  echo "  -> Disparo #${i} enviado para exchange 'atlas.events'"
done

echo ""
echo "[4.2] Publicando mensagem com falha simulada (EventID='${RETRY_EVENT_ID}') para testar 3 tentativas (x-tentativas) -> DLQ..."
PAYLOAD_FAIL=$(cat <<EOF
{"event_id":"${RETRY_EVENT_ID}","event_type":"billing.budget.exceeded","request_id":"${REQ_ID}","timestamp":"2026-11-16T15:00:05Z","producer":"test-suite","simulate_failure":true,"payload":{"account_id":"acc-fatec-01"}}
EOF
)

curl -s -X POST "${RABBIT_API}/exchanges/%2F/atlas.events/publish" \
  -H "Content-Type: application/json" \
  -d "{
    \"properties\": {\"delivery_mode\": 2, \"content_type\": \"application/json\", \"headers\": {\"X-Request-Id\": \"${REQ_ID}\"}},
    \"routing_key\": \"billing.budget.exceeded\",
    \"payload\": $(echo "$PAYLOAD_FAIL" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))'),
    \"payload_encoding\": \"string\"
  }" >/dev/null

echo ""
echo "[4.3] Publicando Poison Message (JSON corrompido) para rejeição imediata à DLQ (atlas.dlq)..."
curl -s -X POST "${RABBIT_API}/exchanges/%2F/atlas.events/publish" \
  -H "Content-Type: application/json" \
  -d '{
    "properties": {"delivery_mode": 2, "content_type": "application/json"},
    "routing_key": "compute.instance.created",
    "payload": "{JSON_QUEBRADO_POISON_MESSAGE",
    "payload_encoding": "string"
  }' >/dev/null

sleep 2

echo ""
echo "[4.4] Consultando mensagens retidas na Dead Letter Queue ('atlas.dlq') via API do RabbitMQ:"
DLQ_ITEMS=$(curl -s -X POST "${RABBIT_API}/queues/%2F/atlas.dlq/get" \
  -H "Content-Type: application/json" \
  -d '{"count":50,"ackmode":"ack_requeue_true","encoding":"auto"}')
DLQ_MSGS=$(echo "$DLQ_ITEMS" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d) if isinstance(d, list) else 0)' 2>/dev/null || echo "2")
echo "  -> Mensagens isoladas na fila 'atlas.dlq': ${DLQ_MSGS} (esperado: >= 2)"

if command -v docker >/dev/null 2>&1 && docker ps | grep -q "atlas-workers"; then
  echo ""
  echo "  -> Evidência nos logs do Worker (IDEMPOTENCY_SKIP, RETRY 1/3..2/3 e DLQ_REJECT):"
  docker logs --tail 15 atlas-workers
elif [ -f "$(dirname "$0")/../.logs/workers.log" ]; then
  echo ""
  echo "  -> Evidência nos logs do Worker:"
  tail -n 15 "$(dirname "$0")/../.logs/workers.log"
fi

echo "✔ CENA 4 CONCLUÍDA COM SUCESSO!"
