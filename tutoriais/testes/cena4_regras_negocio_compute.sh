#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"
VPC_ID="vpc-dev-01"

echo "=========================================================="
echo " CENA 4: Regras de Negócio de Compute (RFC 9457: 422 e 409)"
echo "=========================================================="

echo "[4.1] Criando uma instância inicial 'srv-web-active' (flavor: t3.micro)..."
RESP=$(curl -s -X POST "$GATEWAY_URL/v1/instances" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d "{
    \"name\": \"srv-web-active\",
    \"flavor\": \"t3.micro\",
    \"vpc_id\": \"$VPC_ID\",
    \"security_group_id\": \"sg-default-web\",
    \"account_id\": \"acc-fatec-01\"
  }")
INST_ID=$(echo "$RESP" | jq -r '.id // empty')
echo "      -> Instância criada com ID: $INST_ID (Status: $(echo "$RESP" | jq -r '.status'))"

echo ""
echo "[4.2] REGRA A: Tentar alterar flavor de instância em estado RUNNING (Esperado: 422 Unprocessable Content)..."
STATUS_CODE=$(curl -s -o /tmp/cena4_regra_a.json -w "%{http_code}" -X PATCH "$GATEWAY_URL/v1/instances/$INST_ID" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"flavor": "c5.large"}')
echo "      -> HTTP Status Code recebido: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 422 ]; then
  echo "      -> [PASS] Bloqueio de redimensionamento em RUNNING validado!"
  echo "      -> Detalhe RFC 9457:"
  cat /tmp/cena4_regra_a.json | jq .
else
  echo "      -> [FAIL] Esperado 422, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[4.3] REGRA C: Tentar criar outra instância ativa com o MESMO nome na mesma VPC (Esperado: 409 Conflict)..."
STATUS_CODE=$(curl -s -o /tmp/cena4_regra_c.json -w "%{http_code}" -X POST "$GATEWAY_URL/v1/instances" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d "{
    \"name\": \"srv-web-active\",
    \"flavor\": \"t3.micro\",
    \"vpc_id\": \"$VPC_ID\",
    \"security_group_id\": \"sg-default-web\",
    \"account_id\": \"acc-fatec-01\"
  }")
echo "      -> HTTP Status Code recebido: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 409 ]; then
  echo "      -> [PASS] Bloqueio de duplicidade de nome ativo na mesma VPC validado!"
  echo "      -> Detalhe RFC 9457:"
  cat /tmp/cena4_regra_c.json | jq .
else
  echo "      -> [FAIL] Esperado 409, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[4.4] REGRA B: Ultrapassar a cota máxima de vCPUs da conta (Cota: 8 vCPUs)..."
echo "      Tentando instanciar múltiplas máquinas c5.large (2 vCPUs cada) até ultrapassar 8 vCPUs..."

EXCEEDED=0
for i in 1 2 3 4 5; do
  STATUS_CODE=$(curl -s -o /tmp/cena4_regra_b.json -w "%{http_code}" -X POST "$GATEWAY_URL/v1/instances" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{
      \"name\": \"srv-quota-test-$i\",
      \"flavor\": \"c5.large\",
      \"vpc_id\": \"$VPC_ID\",
      \"security_group_id\": \"sg-default-web\",
      \"account_id\": \"acc-fatec-01\"
    }")
  if [ "$STATUS_CODE" -eq 422 ]; then
    EXCEEDED=1
    echo "      -> [PASS] Criação de instância #$i bloqueada com 422 Unprocessable Content por violação de cota!"
    echo "      -> Detalhe RFC 9457:"
    cat /tmp/cena4_regra_b.json | jq .
    break
  fi
done

if [ "$EXCEEDED" -eq 1 ]; then
  echo ""
  echo ">>> [SUCESSO] CENA 4 VALIDADA: Todas as 3 regras de negócio de Compute validadas com RFC 9457."
  echo "      Limpando instâncias temporárias de teste de cota..."
  for inst_name in srv-web-active srv-quota-test-1 srv-quota-test-2 srv-quota-test-3 srv-quota-test-4 srv-quota-test-5; do
    INST_TO_DEL=$(curl -s -H "X-API-Key: $API_KEY" "$GATEWAY_URL/v1/instances?status=RUNNING" | jq -r ".items[] | select(.name == \"$inst_name\") | .id")
    if [ -n "$INST_TO_DEL" ]; then
      curl -s -X DELETE "$GATEWAY_URL/v1/instances/$INST_TO_DEL" -H "X-API-Key: $API_KEY" >/dev/null
    fi
  done
else
  echo ""
  echo ">>> [FAIL] Não atingiu bloqueio de cota de vCPUs."
  exit 1
fi
