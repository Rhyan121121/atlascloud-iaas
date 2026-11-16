#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"

echo "================================================================================"
echo " CENA 1: AMBIENTE NO AR E TABELA DE PORTAS (ISOLAMENTO DE REDE)"
echo "================================================================================"

echo "[1.1] Verificando saúde do API Gateway na única porta pública (${GATEWAY_URL}/healthz)..."
HTTP_CODE=$(curl -s -o /tmp/c1_health.json -w "%{http_code}" "${GATEWAY_URL}/healthz")
cat /tmp/c1_health.json && echo ""
if [ "$HTTP_CODE" != "200" ]; then
  echo "❌ Falha: Gateway não respondeu 200 OK (recebido: $HTTP_CODE)"
  exit 1
fi

echo ""
echo "[1.2] Tabela de Processos, Portas e Visibilidade de Rede:"
printf "%-28s %-22s %-30s\n" "COMPONENTE" "PORTA / PROTOCOLO" "VISIBILIDADE DE REDE"
echo "--------------------------------------------------------------------------------"
printf "%-28s %-22s %-30s\n" "atlas-gateway"             "0.0.0.0:8080 (HTTP)"   "PÚBLICA (Única entrada externa)"
printf "%-28s %-22s %-30s\n" "atlas-compute-service"     "127.0.0.1:8081 (HTTP)" "INTERNA (Apenas Gateway/BFF)"
printf "%-28s %-22s %-30s\n" "atlas-security-service"    "127.0.0.1:8082 (HTTP)" "INTERNA (Apenas Gateway/BFF)"
printf "%-28s %-22s %-30s\n" "atlas-billing-service"     "127.0.0.1:8083 (HTTP)" "INTERNA (Apenas Gateway/BFF)"
printf "%-28s %-22s %-30s\n" "atlas-graphql-service"     "127.0.0.1:8084 (HTTP)" "INTERNA (Apenas Gateway)"
printf "%-28s %-22s %-30s\n" "atlas-notification-worker" "AMQP 0-9-1 (5672)"     "INTERNA (2 Consumidores Ativos)"
printf "%-28s %-22s %-30s\n" "atlas-rabbitmq"            "5672 / 15672 (Web UI)" "BROKER AMQP + Painel Management"
echo "--------------------------------------------------------------------------------"

if command -v docker >/dev/null 2>&1 && docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null | grep -q "atlas-"; then
  echo ""
  echo "[1.3] Contêineres Docker em execução:"
  docker ps --filter "name=atlas-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
fi

echo "✔ CENA 1 CONCLUÍDA COM SUCESSO!"
