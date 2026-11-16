#!/usr/bin/env bash
set -e

GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"
ACCOUNT_ID="acc-fatec-01"
CUSTOM_REQ_ID="trace-cena-8-graphql-$(date +%s)"

echo "=========================================================="
echo " CENA 8: Consulta Unificada BFF GraphQL (/graphql)"
echo "=========================================================="

echo "[8.1] Executando consulta unificada accountDashboard via GraphQL..."
QUERY='query GetDashboard($accountId: String!) {
  accountDashboard(accountId: $accountId) {
    accountId
    totalInstances
    runningInstances
    totalVCPUs
    instances {
      id
      name
      flavor
      status
      ipPrivado
      ipPublico
      vpcId
      securityGroupId
    }
    securityGroups {
      id
      name
      vpcId
      rules {
        protocol
        startPort
        endPort
        cidrOrigin
      }
    }
    budget {
      currentCost
      budgetLimit
      exceeded
      currency
    }
  }
}'

RESP=$(curl -s -X POST "$GATEWAY_URL/graphql" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-Id: $CUSTOM_REQ_ID" \
  -d "$(jq -n --arg q "$QUERY" --arg acc "$ACCOUNT_ID" '{query: $q, variables: {accountId: $acc}}')")

echo "      -> Resposta GraphQL recebida:"
echo "$RESP" | jq .

ERRORS=$(echo "$RESP" | jq '.errors // empty')
if [ -n "$ERRORS" ]; then
  echo "      -> [FAIL] Erros na execução do GraphQL: $ERRORS"
  exit 1
fi

TOTAL_INSTANCES=$(echo "$RESP" | jq '.data.accountDashboard.totalInstances')
TOTAL_VCPUS=$(echo "$RESP" | jq '.data.accountDashboard.totalVCPUs')
BUDGET_EXCEEDED=$(echo "$RESP" | jq '.data.accountDashboard.budget.exceeded')

echo ""
echo "      -> Total de Instâncias Retornadas: $TOTAL_INSTANCES"
echo "      -> Total de vCPUs em Uso: $TOTAL_VCPUS"
echo "      -> Alerta de Fatura Excedida: $BUDGET_EXCEEDED"
echo "      -> [PASS] Uma única consulta GraphQL substituiu 3 chamadas REST (Compute, Security e Billing)!"

echo ""
echo ">>> [SUCESSO] CENA 8 VALIDADA: Servidor GraphQL agregador BFF 100% operacional."
