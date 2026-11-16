#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
ACCOUNT_ID="acc-fatec-01"

echo "=========================================================="
echo " CENA 3: Rota Agregada do Dashboard com Degradação Graciosa"
echo "=========================================================="

echo "[3.1] Chamando rota agregada GET /api/v1/dashboard/$ACCOUNT_ID..."
RESP=$(curl -s -H "X-API-Key: cli-terraform" -H "X-Request-Id: trace-cena-3-dashboard" "$GATEWAY_URL/api/v1/dashboard/$ACCOUNT_ID")

echo "      -> Resposta agregada recebida:"
echo "$RESP" | jq .

STATUS=$(echo "$RESP" | jq -r '.status')
DEGRADED_COUNT=$(echo "$RESP" | jq '.degraded_services | length')

echo ""
echo "      -> Status do Dashboard: $STATUS"
echo "      -> Serviços Degradados: $DEGRADED_COUNT"

if [ "$STATUS" = "HEALTHY" ] || [ "$STATUS" = "DEGRADED" ]; then
  echo "      -> [PASS] Agregação executada com sucesso com tratamento gracioso de falhas."
else
  echo "      -> [FAIL] Resposta inesperada do dashboard: $STATUS"
  exit 1
fi

echo ""
echo ">>> [SUCESSO] CENA 3 VALIDADA: Dashboard agregado resiliente com degradação graciosa."
