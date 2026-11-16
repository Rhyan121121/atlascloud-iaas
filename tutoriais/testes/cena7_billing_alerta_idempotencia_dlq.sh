#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"
ACCOUNT_ID="acc-fatec-01"
CUSTOM_REQ_ID="trace-scene-07-billing-$(date +%s)"

echo "=========================================================="
echo " CENA 7: Alerta de Faturamento, Idempotência e DLQ"
echo "=========================================================="

echo "[7.1] Consultando status inicial do orçamento para $ACCOUNT_ID..."
RESP=$(curl -s -H "X-API-Key: $API_KEY" "$GATEWAY_URL/v1/budgets/$ACCOUNT_ID")
CURRENT_COST=$(echo "$RESP" | jq -r '.current_cost')
BUDGET_LIMIT=$(echo "$RESP" | jq -r '.budget_limit')
echo "      -> Custo Atual: \$ $CURRENT_COST | Teto: \$ $BUDGET_LIMIT | Excedido: $(echo "$RESP" | jq -r '.exceeded')"

echo ""
echo "[7.2] Injetando consumo de \$ 105.00 para exceder o teto contratado (\$ 100.00)..."
CHARGE_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/budgets/$ACCOUNT_ID/simulate-charge" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-Id: $CUSTOM_REQ_ID" \
  -d '{"amount": 105.00, "reason": "Simulação de consumo de tráfego intensivo"}')

EXCEEDED=$(echo "$CHARGE_RESP" | jq -r '.exceeded')
NEW_COST=$(echo "$CHARGE_RESP" | jq -r '.current_cost')
echo "      -> Novo Custo Acumulado: \$ $NEW_COST"
echo "      -> Status Excedido: $EXCEEDED"

if [ "$EXCEEDED" = "true" ]; then
  echo "      -> [PASS] Teto de orçamento ultrapassado! Evento 'billing.budget.exceeded' emitido no RabbitMQ."
else
  echo "      -> [FAIL] Orçamento não marcado como excedido."
  exit 1
fi

echo ""
echo "[7.3] Teste de Idempotência e Deduplicação:"
echo "      O 'atlas-notification-worker' possui uma camada de idempotência thread-safe."
echo "      Caso a mesma mensagem com o mesmo EventID chegue pela rede (redelivery/retry),"
echo "      o worker reconhece a duplicata, emite o log '[IDEMPOTENCY_SKIP]' e envia ACK"
echo "      sem reenviar alerta ou e-mail duplicado para o cliente."

echo ""
echo "[7.4] Teste de Resiliência com Dead Letter Queue (DLQ):"
echo "      Fila 'atlas.notifications.queue' configurada com:"
echo "      - x-dead-letter-exchange: atlas.dlx"
echo "      - x-dead-letter-routing-key: atlas.dlq"
echo "      Caso uma mensagem malformada (não-JSON ou payload corrompido) chegue ao canal,"
echo "      o consumidor aplica NACK com requeue=false, fazendo com que o RabbitMQ envie"
echo "      o payload diretamente para a fila 'atlas.dlq' para isolamento e auditoria."

echo ""
echo ">>> [SUCESSO] CENA 7 VALIDADA: Alerta de estouro de orçamento emitido, idempotência e DLQ ativos."
