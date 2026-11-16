#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"

echo "=========================================================="
echo " CENA 5: Regras de Firewall e Bloqueio de Deleção (422 e 409)"
echo "=========================================================="

echo "[5.1] Testando validação de portas inválidas (< 1 ou > 65535) (Esperado: 422 Unprocessable Content)..."
STATUS_CODE=$(curl -s -o /tmp/cena5_invalid_port.json -w "%{http_code}" -X POST "$GATEWAY_URL/v1/security-groups" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "name": "sg-invalid-ports",
    "vpc_id": "vpc-dev-01",
    "account_id": "acc-fatec-01",
    "rules": [
      {"protocol": "TCP", "start_port": 0, "end_port": 80, "cidr_origin": "0.0.0.0/0"}
    ]
  }')
echo "      -> HTTP Status Code recebido: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 422 ]; then
  echo "      -> [PASS] Bloqueio de porta inválida (< 1) validado com sucesso!"
  echo "      -> Detalhe RFC 9457:"
  cat /tmp/cena5_invalid_port.json | jq .
else
  echo "      -> [FAIL] Esperado 422, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[5.2] Criando Security Group válido 'sg-database-locked' para teste de dependência..."
SG_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/security-groups" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "name": "sg-database-locked",
    "vpc_id": "vpc-dev-01",
    "account_id": "acc-fatec-01",
    "rules": [
      {"protocol": "TCP", "start_port": 5432, "end_port": 5432, "cidr_origin": "10.0.0.0/16"}
    ]
  }')
SG_ID=$(echo "$SG_RESP" | jq -r '.id // empty')
echo "      -> Security Group criado: $SG_ID"

echo ""
echo "[5.3] Criando uma Instância vinculada a este Security Group..."
INST_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/instances" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d "{
    \"name\": \"db-instance-tied-to-sg\",
    \"flavor\": \"t3.micro\",
    \"vpc_id\": \"vpc-dev-01\",
    \"security_group_id\": \"$SG_ID\",
    \"account_id\": \"acc-fatec-01\"
  }")
INST_ID=$(echo "$INST_RESP" | jq -r '.id // empty')
echo "      -> Instância vinculada criada: $INST_ID"

echo ""
echo "[5.4] Tentando EXCLUIR o Security Group em uso por instância ativa (Esperado: 409 Conflict)..."
STATUS_CODE=$(curl -s -o /tmp/cena5_conflict.json -w "%{http_code}" -X DELETE "$GATEWAY_URL/v1/security-groups/$SG_ID" \
  -H "X-API-Key: $API_KEY")
echo "      -> HTTP Status Code recebido: $STATUS_CODE"
if [ "$STATUS_CODE" -eq 409 ]; then
  echo "      -> [PASS] Exclusão bloqueada por vínculo com instâncias ativas (409 Conflict)!"
  echo "      -> Detalhe RFC 9457:"
  cat /tmp/cena5_conflict.json | jq .
else
  echo "      -> [FAIL] Esperado 409, recebido $STATUS_CODE"
  exit 1
fi

echo ""
echo "[5.5] Limpando ambiente: terminando instância para permitir exclusão posterior..."
curl -s -X DELETE "$GATEWAY_URL/v1/instances/$INST_ID" -H "X-API-Key: $API_KEY" >/dev/null

echo ">>> [SUCESSO] CENA 5 VALIDADA: Firewall validado e bloqueio de integridade relacional ativo."
