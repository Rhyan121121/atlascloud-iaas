#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_KEY="cli-terraform"

echo "================================================================================"
echo " CENA 2: API REST PELO GATEWAY (CRUD, LOCATION, PAGINAÇÃO E REGRAS DE NEGÓCIO)"
echo "================================================================================"

# Limpar instâncias prévias da conta de teste para garantir idempotência em múltiplas execuções
EXISTING_IDS=$(curl -s "${GATEWAY_URL}/api/v1/instances?limit=50" -H "X-API-Key: ${API_KEY}" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    for item in d.get("items", []):
        print(item["id"])
except Exception:
    pass
' 2>/dev/null || true)

for eid in $EXISTING_IDS; do
  curl -s -X DELETE "${GATEWAY_URL}/api/v1/instances/${eid}?purge=true" -H "X-API-Key: ${API_KEY}" >/dev/null || true
done
curl -s -X POST "${GATEWAY_URL}/api/v1/instances/purge" -H "X-API-Key: ${API_KEY}" >/dev/null || true

echo "[2.1] Criando Security Group para a VPC..."
SG_RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/security-groups" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sg-web-cluster",
    "vpc_id": "vpc-prod-01",
    "account_id": "acc-fatec-01",
    "rules": [
      {"protocol": "TCP", "start_port": 80, "end_port": 80, "cidr_origin": "0.0.0.0/0"},
      {"protocol": "TCP", "start_port": 443, "end_port": 443, "cidr_origin": "0.0.0.0/0"}
    ]
  }')
SG_ID=$(echo "$SG_RESP" | grep -o '"id":"[^"]*"' | head -n1 | cut -d'"' -f4)
echo "  -> Security Group criado: ID=${SG_ID}"

echo ""
echo "[2.2] Criando Instância Virtual via POST /api/v1/instances (201 Created + Location reescrito)..."
HEADERS_FILE="/tmp/c2_post_headers.txt"
BODY_FILE="/tmp/c2_post_body.json"
HTTP_CODE=$(curl -s -D "$HEADERS_FILE" -o "$BODY_FILE" -w "%{http_code}" \
  -X POST "${GATEWAY_URL}/api/v1/instances" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"vm-app-prod-01\",
    \"flavor\": \"c5.large\",
    \"vpc_id\": \"vpc-prod-01\",
    \"security_group_id\": \"${SG_ID}\",
    \"account_id\": \"acc-fatec-01\"
  }")
LOC_HEADER=$(grep -i "^Location:" "$HEADERS_FILE" | tr -d '\r')
INST_ID=$(cat "$BODY_FILE" | grep -o '"id":"[^"]*"' | head -n1 | cut -d'"' -f4)
echo "  -> Status HTTP: ${HTTP_CODE} (esperado: 201)"
echo "  -> Cabeçalho reescrito pelo Gateway: ${LOC_HEADER}"
echo "  -> Instância criada (UUIDv7): ${INST_ID}"
[ "$HTTP_CODE" = "201" ] || exit 1

echo ""
echo "[2.3] Lendo instância criada (GET /api/v1/instances/${INST_ID}) e listando com paginação/filtro..."
HTTP_GET=$(curl -s -o /dev/null -w "%{http_code}" "${GATEWAY_URL}/api/v1/instances/${INST_ID}" -H "X-API-Key: ${API_KEY}")
HTTP_LIST=$(curl -s -o /tmp/c2_list.json -w "%{http_code}" "${GATEWAY_URL}/api/v1/instances?status=RUNNING&flavor=c5.large&pagina=1&limite=5&ordenar=name&ordem=asc" -H "X-API-Key: ${API_KEY}")
echo "  -> GET item: ${HTTP_GET} | GET lista filtrada/paginada: ${HTTP_LIST}"
cat /tmp/c2_list.json | head -c 320 && echo "..."

echo ""
echo "[2.4] Regra de Negócio 1: Tentando alterar flavor de instância em RUNNING (esperado: 422 Problem Details)..."
HTTP_R1=$(curl -s -o /tmp/c2_r1.json -w "%{http_code}" \
  -X PATCH "${GATEWAY_URL}/api/v1/instances/${INST_ID}" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"flavor": "t3.micro"}')
echo "  -> Status HTTP: ${HTTP_R1}"
cat /tmp/c2_r1.json && echo ""
[ "$HTTP_R1" = "422" ] || exit 1

echo ""
echo "[2.5] Regra de Negócio 2: Tentando criar instância com nome duplicado na mesma VPC (esperado: 409 Conflict)..."
HTTP_R2=$(curl -s -o /tmp/c2_r2.json -w "%{http_code}" \
  -X POST "${GATEWAY_URL}/api/v1/instances" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"vm-app-prod-01\",
    \"flavor\": \"t3.micro\",
    \"vpc_id\": \"vpc-prod-01\",
    \"security_group_id\": \"${SG_ID}\",
    \"account_id\": \"acc-fatec-01\"
  }")
echo "  -> Status HTTP: ${HTTP_R2}"
cat /tmp/c2_r2.json && echo ""
[ "$HTTP_R2" = "409" ] || exit 1

echo ""
echo "[2.6] Regra de Negócio 3: Tentando excluir Security Group vinculado a instância ativa (esperado: 409 Conflict)..."
HTTP_R3=$(curl -s -o /tmp/c2_r3.json -w "%{http_code}" \
  -X DELETE "${GATEWAY_URL}/api/v1/security-groups/${SG_ID}" \
  -H "X-API-Key: ${API_KEY}")
echo "  -> Status HTTP: ${HTTP_R3}"
cat /tmp/c2_r3.json && echo ""
[ "$HTTP_R3" = "409" ] || exit 1

echo ""
echo "[2.7] Parando instância (PATCH -> STOPPED), substituindo via PUT (200 OK) e removendo via DELETE (204 No Content)..."
curl -s -X PATCH "${GATEWAY_URL}/api/v1/instances/${INST_ID}" \
  -H "X-API-Key: ${API_KEY}" -H "Content-Type: application/json" -d '{"status": "STOPPED"}' >/dev/null
HTTP_PUT=$(curl -s -o /dev/null -w "%{http_code}" \
  -X PUT "${GATEWAY_URL}/api/v1/instances/${INST_ID}" \
  -H "X-API-Key: ${API_KEY}" -H "Content-Type: application/json" \
  -d "{
    \"name\": \"vm-app-prod-01-v2\",
    \"flavor\": \"c5.large\",
    \"vpc_id\": \"vpc-prod-01\",
    \"security_group_id\": \"${SG_ID}\",
    \"account_id\": \"acc-fatec-01\"
  }")
HTTP_DEL=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "${GATEWAY_URL}/api/v1/instances/${INST_ID}?purge=true" \
  -H "X-API-Key: ${API_KEY}")
echo "  -> PUT /api/v1/instances/${INST_ID} retornou: ${HTTP_PUT} (esperado: 200) | DELETE retornou: ${HTTP_DEL} (esperado: 204)"
[ "$HTTP_PUT" = "200" ] || exit 1
[ "$HTTP_DEL" = "204" ] || exit 1

echo "✔ CENA 2 CONCLUÍDA COM SUCESSO!"
