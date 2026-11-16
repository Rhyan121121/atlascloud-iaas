#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"

echo "================================================================================"
echo " CENA 6: PROTEÇÕES DO API GATEWAY (401, 403, 429, 504, 502 E 503 DISJUNTOR)"
echo "================================================================================"

echo "[6.1] Chamada sem X-API-Key (esperado: 401 Unauthorized em Problem Details)..."
C401=$(curl -s -o /tmp/c6_401.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/instances")
echo "  -> Status HTTP: ${C401}"
cat /tmp/c6_401.json && echo ""
[ "$C401" = "401" ] || exit 1

echo ""
echo "[6.2] Chamada com chave 'parceiro-auditoria' em rota restrita /api/v1/instances (esperado: 403 Forbidden - Ponto Extra)..."
C403=$(curl -s -o /tmp/c6_403.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/instances" -H "X-API-Key: parceiro-auditoria")
echo "  -> Status HTTP: ${C403}"
cat /tmp/c6_403.json && echo ""
[ "$C403" = "403" ] || exit 1

echo ""
echo "[6.3] Estourando Rate Limit do plano 'console-web' (40 req/min -> esperado: 429 Too Many Requests + Retry-After)..."
GOT_429=0
for i in $(seq 1 45); do
  CODE=$(curl -s -D /tmp/c6_rl_h.txt -o /tmp/c6_429.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/budgets/acc-fatec-01" -H "X-API-Key: console-web")
  if [ "$CODE" = "429" ]; then
    GOT_429=1
    RETRY_H=$(grep -i "^Retry-After:" /tmp/c6_rl_h.txt | tr -d '\r')
    echo "  -> Requisição #${i} bloqueada com HTTP 429! Cabeçalho: ${RETRY_H}"
    cat /tmp/c6_429.json && echo ""
    break
  fi
done
[ "$GOT_429" = "1" ] || exit 1

echo ""
echo "[6.4] Provocando Timeout de 2s no Gateway via ?atraso=2600 (esperado: 504 Gateway Timeout - Restrição EARS [R5])..."
C504=$(curl -s -o /tmp/c6_504.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/instances?atraso=2600" -H "X-API-Key: cli-terraform")
echo "  -> Status HTTP: ${C504}"
cat /tmp/c6_504.json && echo ""
[ "$C504" = "504" ] || exit 1

echo ""
echo "[6.5] Simulando queda do serviço Security (502 Bad Gateway x3 -> abertura do Disjuntor 503 Service Unavailable)..."
STOPPED_MODE=""
if command -v docker >/dev/null 2>&1 && docker ps | grep -q "atlas-security"; then
  docker stop atlas-security >/dev/null
  STOPPED_MODE="docker"
elif [ -f "$(dirname "$0")/../.pids/security.pid" ]; then
  kill "$(cat "$(dirname "$0")/../.pids/security.pid")" 2>/dev/null || true
  STOPPED_MODE="local"
fi

for attempt in 1 2 3; do
  CODE=$(curl -s -o /dev/null -w "%{http_code}" "${GATEWAY_URL}/api/v1/security-groups" -H "X-API-Key: cli-terraform")
  echo "  -> Falha consecutiva #${attempt}: HTTP ${CODE} (502 Bad Gateway)"
done

C503=$(curl -s -D /tmp/c6_cb_h.txt -o /tmp/c6_503.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/security-groups" -H "X-API-Key: cli-terraform")
CB_RETRY=$(grep -i "^Retry-After:" /tmp/c6_cb_h.txt | tr -d '\r')
echo "  -> 4ª chamada com Disjuntor Aberto (Fail-Fast imediato): HTTP ${C503} | ${CB_RETRY}"
cat /tmp/c6_503.json && echo ""
[ "$C503" = "503" ] || exit 1

echo ""
echo "[6.6] Religando serviço Security e aguardando fechamento automático do Disjuntor (Half-Open -> Closed)..."
if [ "$STOPPED_MODE" = "docker" ]; then
  docker start atlas-security >/dev/null
elif [ "$STOPPED_MODE" = "local" ]; then
  PORT=8082 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" nohup "$(dirname "$0")/../bin/atlas-security" >> "$(dirname "$0")/../.logs/security.log" 2>&1 &
  echo $! > "$(dirname "$0")/../.pids/security.pid"
fi
sleep 9
C200=$(curl -s -o /dev/null -w "%{http_code}" "${GATEWAY_URL}/api/v1/security-groups" -H "X-API-Key: cli-terraform")
echo "  -> Serviço recuperado e Disjuntor fechado novamente: HTTP ${C200} (esperado: 200)"
[ "$C200" = "200" ] || exit 1

echo "✔ CENA 6 CONCLUÍDA COM SUCESSO!"
