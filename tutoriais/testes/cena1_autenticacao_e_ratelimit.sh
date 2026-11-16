#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
echo "=========================================================="
echo " CENA 1: Autenticação via X-API-Key e Rate Limiting (429)"
echo "=========================================================="

echo "[1.1] Testando requisição SEM cabeçalho X-API-Key (Esperado: 401 Unauthorized)..."
STATUS_CODE=$(curl -s -o /tmp/cena1_401.json -w "%{http_code}" "$GATEWAY_URL/v1/instances")
echo "      -> HTTP Status Code: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 401 ]; then
  echo "      -> [PASS] 401 recebido conforme esperado."
  echo "      -> Corpo RFC 9457:"
  cat /tmp/cena1_401.json | jq .
else
  echo "      -> [FAIL] Esperado 401, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[1.2] Testando requisição com X-API-Key INVÁLIDA (Esperado: 401 Unauthorized)..."
STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: chave-falsa-123" "$GATEWAY_URL/v1/instances")
echo "      -> HTTP Status Code: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 401 ]; then
  echo "      -> [PASS] Bloqueio de chave inválida validado."
else
  echo "      -> [FAIL] Esperado 401, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[1.3] Testando requisição com perfil 'cli-terraform' (Esperado: 200 OK)..."
STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: cli-terraform" "$GATEWAY_URL/v1/instances")
echo "      -> HTTP Status Code: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 200 ]; then
  echo "      -> [PASS] Perfil cli-terraform aceito com sucesso."
else
  echo "      -> [FAIL] Esperado 200, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[1.4] Testando estouro de Rate Limit para o perfil 'console-web' (Limite: 40 req/min)..."
echo "      Disparando rajada de requisições rápidas para forçar HTTP 429 Too Many Requests..."

HIT_429=0
RETRY_AFTER=""
for i in $(seq 1 45); do
  RESP_HEADERS=$(curl -s -D - -o /tmp/cena1_resp.json -H "X-API-Key: console-web" "$GATEWAY_URL/v1/instances")
  CODE=$(echo "$RESP_HEADERS" | grep "HTTP/" | awk '{print $2}' | tail -n1)
  if [ "$CODE" -eq 429 ]; then
    HIT_429=1
    RETRY_AFTER=$(echo "$RESP_HEADERS" | grep -i "Retry-After" | awk '{print $2}' | tr -d '\r')
    echo "      -> [PASS] Requisição #$i foi bloqueada com HTTP 429 Too Many Requests!"
    echo "      -> Cabeçalho Retry-After: $RETRY_AFTER segundos"
    echo "      -> Resposta RFC 9457:"
    cat /tmp/cena1_resp.json | jq .
    break
  fi
done

if [ "$HIT_429" -eq 1 ]; then
  echo ""
  echo ">>> [SUCESSO] CENA 1 VALIDADA: Autenticação e Rate Limiting RFC 9457 funcionais."
else
  echo ""
  echo ">>> [FAIL] Rate limiting não bloqueou após 45 requisições."
  exit 1
fi
