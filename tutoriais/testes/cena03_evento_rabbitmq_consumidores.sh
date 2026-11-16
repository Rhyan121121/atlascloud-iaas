#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_KEY="cli-terraform"
REQ_ID="req-cena3-evento-$(date +%s)"

echo "================================================================================"
echo " CENA 3: EVENTO ASSÍNCRONO NO RABBITMQ E ROTEAMENTO PARA 2 CONSUMIDORES"
echo "================================================================================"

echo "[3.1] Criando instância com X-Request-Id='${REQ_ID}' (publica compute.instance.created)..."
RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/instances" \
  -H "X-API-Key: ${API_KEY}" \
  -H "X-Request-Id: ${REQ_ID}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "vm-eventos-rabbit-01",
    "flavor": "t3.micro",
    "vpc_id": "vpc-eventos-01",
    "security_group_id": "sg-default",
    "account_id": "acc-fatec-01"
  }')
INST_ID=$(echo "$RESP" | grep -o '"id":"[^"]*"' | head -n1 | cut -d'"' -f4)
echo "  -> Resposta 201 imediata (assíncrona): Instância ID=${INST_ID}"

echo ""
echo "[3.2] Registrando cobrança de US$ 15.00 abaixo do teto (publica billing.charge.recorded -> só Consumidor 2 Auditoria recebe)..."
curl -s -X POST "${GATEWAY_URL}/api/v1/budgets/acc-fatec-01/reset" -H "X-API-Key: ${API_KEY}" >/dev/null
curl -s -X POST "${GATEWAY_URL}/api/v1/budgets/acc-fatec-01/simulate-charge" \
  -H "X-API-Key: ${API_KEY}" \
  -H "X-Request-Id: ${REQ_ID}-charge" \
  -H "Content-Type: application/json" \
  -d '{"amount": 15.00}' >/dev/null

echo ""
echo "[3.3] Aguardando processamento assíncrono pelos 2 consumidores (1s)..."
sleep 1

if command -v docker >/dev/null 2>&1 && docker ps | grep -q "atlas-workers"; then
  echo "  -> Logs dos 2 Consumidores (Notificações e Auditoria) no container atlas-workers:"
  docker logs --tail 12 atlas-workers
elif [ -f "$(dirname "$0")/../.logs/workers.log" ]; then
  echo "  -> Logs dos 2 Consumidores (Notificações e Auditoria):"
  tail -n 12 "$(dirname "$0")/../.logs/workers.log"
fi

# Limpar instância criada na cena
if [ -n "$INST_ID" ]; then
  curl -s -X DELETE "${GATEWAY_URL}/api/v1/instances/${INST_ID}?purge=true" -H "X-API-Key: ${API_KEY}" >/dev/null || true
fi

echo "✔ CENA 3 CONCLUÍDA COM SUCESSO!"
