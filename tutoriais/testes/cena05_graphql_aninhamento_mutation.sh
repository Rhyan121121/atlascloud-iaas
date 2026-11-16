#!/usr/bin/env bash
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_KEY="cli-terraform"
REQ_ID="req-cena5-graphql-$(date +%s)"

echo "================================================================================"
echo " CENA 5: SERVIDOR GRAPHQL (ANINHAMENTO 3 NÍVEIS, DATALOADER, FILTRO E MUTATION)"
echo "================================================================================"

# Garante Security Group com regra de firewall ativa para demonstrar o aninhamento de 3 níveis
SG_RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/security-groups" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"name":"sg-graphql-web","vpc_id":"vpc-prod-01","description":"SG para demo GraphQL 3 niveis"}')
SG_ID=$(echo "$SG_RESP" | grep -o '"id":"[^"]*"' | head -n1 | cut -d'"' -f4)
curl -s -X POST "${GATEWAY_URL}/api/v1/security-groups/${SG_ID}/rules" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"TCP","start_port":443,"end_port":443,"cidr_origin":"10.0.0.0/8","action":"ALLOW"}' >/dev/null

echo "[5.1] Executando Mutation 'createInstance' com input CreateInstanceInput via /api/graphql..."
MUT_OK=$(curl -s -X POST "${GATEWAY_URL}/api/graphql" \
  -H "X-API-Key: ${API_KEY}" \
  -H "X-Request-Id: ${REQ_ID}" \
  -H "Content-Type: application/json" \
  -d "{
    \"query\": \"mutation CriarVM(\$input: CreateInstanceInput!) { createInstance(input: \$input) { id name flavor status ipPrivado vpcId securityGroupId } }\",
    \"variables\": {
      \"input\": {
        \"name\": \"vm-graphql-bff-${REQ_ID}\",
        \"flavor\": \"t3.micro\",
        \"vpcId\": \"vpc-prod-01\",
        \"securityGroupId\": \"${SG_ID}\",
        \"accountId\": \"acc-fatec-01\"
      }
    }
  }")
echo "  -> Resposta da Mutation:"
echo "$MUT_OK"
GQL_INST_ID=$(echo "$MUT_OK" | python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("data") or {}).get("createInstance", {}).get("id", ""))' 2>/dev/null || true)

echo ""
echo "[5.2] Executando Consulta Agregada 'accountDashboard' com aninhamento de 3 níveis (AccountDashboard -> Instance -> SecurityGroup -> FirewallRule)..."
DASH_GQL=$(curl -s -X POST "${GATEWAY_URL}/api/graphql" \
  -H "X-API-Key: ${API_KEY}" \
  -H "X-Request-Id: ${REQ_ID}" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "query PainelUnificado($acc: String!) { accountDashboard(accountId: $acc) { accountId totalInstances runningInstances totalVCPUs restCallsAvoided budget { currentCost budgetLimit exceeded } instances { id name flavor status securityGroup { id name rules { protocol startPort endPort cidrOrigin } } } } }",
    "variables": { "acc": "acc-fatec-01" }
  }')
echo "$DASH_GQL" | python3 -m json.tool | head -n 35
AVOIDED=$(echo "$DASH_GQL" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["data"]["accountDashboard"]["restCallsAvoided"])')
echo "  -> Chamadas REST substituídas por esta única consulta GraphQL: ${AVOIDED} chamadas REST!"

echo ""
echo "[5.3] Testando tratamento de erro na Mutation GraphQL com flavor inválido (esperado: extensions.code = BAD_USER_INPUT)..."
MUT_ERR=$(curl -s -X POST "${GATEWAY_URL}/api/graphql" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation ErroValidacao($input: CreateInstanceInput!) { createInstance(input: $input) { id name } }",
    "variables": {
      "input": {
        "name": "vm-invalida",
        "flavor": "m5.24xlarge-inexistente",
        "vpcId": "vpc-prod-01",
        "securityGroupId": "sg-default"
      }
    }
  }')
echo "  -> Resposta com erro estruturado (GraphQLError + extensions.code):"
echo "$MUT_ERR"

if [ -n "$GQL_INST_ID" ]; then
  curl -s -X DELETE "${GATEWAY_URL}/api/v1/instances/${GQL_INST_ID}?purge=true" -H "X-API-Key: ${API_KEY}" >/dev/null || true
fi

echo "✔ CENA 5 CONCLUÍDA COM SUCESSO!"
