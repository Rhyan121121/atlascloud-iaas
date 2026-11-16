#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_KEY="cli-terraform"
TRACE_ID="trace-fatec-e2e-$(date +%s)"

echo "================================================================================"
echo " CENA 8: FECHAMENTO — RASTREAMENTO PONTA A PONTA COM O MESMO X-REQUEST-ID"
echo "================================================================================"

echo "[8.1] Disparando operação no Gateway com X-Request-Id='${TRACE_ID}'..."
RESP_HEADERS="/tmp/c8_headers.txt"
RESP_BODY="/tmp/c8_body.json"
curl -s -D "$RESP_HEADERS" -o "$RESP_BODY" \
  -X POST "${GATEWAY_URL}/api/v1/instances" \
  -H "X-API-Key: ${API_KEY}" \
  -H "X-Request-Id: ${TRACE_ID}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "vm-trace-ponta-a-ponta",
    "flavor": "t3.micro",
    "vpc_id": "vpc-trace-01",
    "security_group_id": "sg-default",
    "account_id": "acc-fatec-01"
  }'

INST_ID=$(cat "$RESP_BODY" | grep -o '"id":"[^"]*"' | head -n1 | cut -d'"' -f4)
sleep 1

echo ""
echo "[8.2] Comprovando o MESMO X-Request-Id ('${TRACE_ID}') nos 3 componentes da arquitetura:"
echo "  1) No cabeçalho de resposta do API Gateway:"
grep -i "^X-Request-Id:" "$RESP_HEADERS" | sed 's/^/     /'

if command -v docker >/dev/null 2>&1 && docker ps | grep -q "atlas-gateway"; then
  echo "  2) Nos logs do API Gateway (atlas-gateway):"
  docker logs atlas-gateway 2>&1 | grep "${TRACE_ID}" | sed 's/^/     /' || true
  echo "  3) Nos logs da API REST (atlas-compute):"
  docker logs atlas-compute 2>&1 | grep "${TRACE_ID}" | sed 's/^/     /' || true
  echo "  4) Nos logs dos 2 Consumidores RabbitMQ (atlas-workers):"
  docker logs atlas-workers 2>&1 | grep "${TRACE_ID}" | sed 's/^/     /' || true
elif [ -d "$(dirname "$0")/../.logs" ]; then
  LOG_D="$(dirname "$0")/../.logs"
  echo "  2) Nos logs do API Gateway:"
  grep "${TRACE_ID}" "$LOG_D/gateway.log" | sed 's/^/     /' || true
  echo "  3) Nos logs da API REST (Compute):"
  grep "${TRACE_ID}" "$LOG_D/compute.log" | sed 's/^/     /' || true
  echo "  4) Nos logs dos 2 Consumidores RabbitMQ (Notificações + Auditoria):"
  grep "${TRACE_ID}" "$LOG_D/workers.log" | sed 's/^/     /' || true
fi

if [ -n "$INST_ID" ]; then
  curl -s -X DELETE "${GATEWAY_URL}/api/v1/instances/${INST_ID}?purge=true" -H "X-API-Key: ${API_KEY}" >/dev/null || true
fi

echo "✔ CENA 8 CONCLUÍDA COM SUCESSO!"
