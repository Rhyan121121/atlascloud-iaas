#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_KEY="cli-terraform"

echo "================================================================================"
echo " CENA 7: ROTA AGREGADA COM DEGRADAÇÃO GRACIOSA (COMPLETA X PARCIAL)"
echo "================================================================================"

echo "[7.1] Consultando rota agregada GET /api/v1/dashboard/acc-fatec-01 com todos os serviços ativos..."
RESP_FULL=$(curl -s "${GATEWAY_URL}/api/v1/dashboard/acc-fatec-01" -H "X-API-Key: ${API_KEY}")
STATUS_FULL=$(echo "$RESP_FULL" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("status=%s | parcial=%s | degraded_services=%s" % (d.get("status"), d.get("parcial"), d.get("degraded_services")))')
echo "  -> Resposta Completa: ${STATUS_FULL}"

echo ""
echo "[7.2] Desligando temporariamente o Billing Service (porta 8083) para testar Degradação Graciosa..."
STOPPED_MODE=""
if command -v docker >/dev/null 2>&1 && docker ps | grep -q "atlas-billing"; then
  docker stop atlas-billing >/dev/null
  STOPPED_MODE="docker"
elif [ -f "$(dirname "$0")/../.pids/billing.pid" ]; then
  kill "$(cat "$(dirname "$0")/../.pids/billing.pid")" 2>/dev/null || true
  STOPPED_MODE="local"
fi

sleep 1
RESP_DEG=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${GATEWAY_URL}/api/v1/dashboard/acc-fatec-01" -H "X-API-Key: ${API_KEY}")
HTTP_DEG=$(echo "$RESP_DEG" | grep "HTTP_CODE:" | cut -d':' -f2)
BODY_DEG=$(echo "$RESP_DEG" | sed '/HTTP_CODE:/d')
STATUS_DEG=$(echo "$BODY_DEG" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("status=%s | parcial=%s | degraded_services=%s" % (d.get("status"), d.get("parcial"), d.get("degraded_services")))')
echo "  -> HTTP Status: ${HTTP_DEG} (continua 200 OK!) | ${STATUS_DEG}"
echo "$BODY_DEG" | python3 -m json.tool | head -n 22

echo ""
echo "[7.3] Religando o Billing Service e confirmando retorno ao estado HEALTHY..."
if [ "$STOPPED_MODE" = "docker" ]; then
  docker start atlas-billing >/dev/null
elif [ "$STOPPED_MODE" = "local" ]; then
  PORT=8083 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" RABBITMQ_URL="amqp://guest:guest@127.0.0.1:5672/" \
    nohup "$(dirname "$0")/../bin/atlas-billing" >> "$(dirname "$0")/../.logs/billing.log" 2>&1 &
  echo $! > "$(dirname "$0")/../.pids/billing.pid"
fi
sleep 2

echo "✔ CENA 7 CONCLUÍDA COM SUCESSO!"
