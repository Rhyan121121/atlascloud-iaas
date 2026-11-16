#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
cd "$DIR"

ACTION="${1:-apply}"
GATEWAY_URL="http://localhost:8080"
API_KEY="cli-terraform"
ACCOUNT_ID="acc-fatec-01"
VPC_ID="vpc-atlas-prod"
STATE_FILE="$DIR/.terraform_state.json"

echo "=========================================================="
echo " AtlasCloud IaaS - Orquestrador Declarativo Terraform"
echo " Ação: $ACTION | Gateway: $GATEWAY_URL"
echo "=========================================================="

if command -v terraform >/dev/null 2>&1; then
  echo "[Terraform CLI Detectado] Executando comandos nativos do Terraform..."
  if [ "$ACTION" = "init" ]; then
    terraform init
  elif [ "$ACTION" = "apply" ]; then
    terraform init -input=false
    terraform apply -auto-approve
  elif [ "$ACTION" = "destroy" ]; then
    terraform destroy -auto-approve
  fi
  exit 0
fi

# Fallback autônomo e declarativo caso o host não tenha o binário terraform instalado
if [ "$ACTION" = "apply" ]; then
  echo "[1/4] Provisionando Security Group Web (sg-web-tier)..."
  SG_WEB_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/security-groups" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{
      \"name\": \"sg-web-tier\",
      \"vpc_id\": \"$VPC_ID\",
      \"account_id\": \"$ACCOUNT_ID\",
      \"rules\": [
        {\"protocol\": \"TCP\", \"start_port\": 80, \"end_port\": 80, \"cidr_origin\": \"0.0.0.0/0\"},
        {\"protocol\": \"TCP\", \"start_port\": 443, \"end_port\": 443, \"cidr_origin\": \"0.0.0.0/0\"}
      ]
    }")
  SG_WEB_ID=$(echo "$SG_WEB_RESP" | jq -r '.id // empty')
  if [ -z "$SG_WEB_ID" ]; then
    echo "Erro ao criar SG Web: $SG_WEB_RESP"
    exit 1
  fi
  echo "      -> SG Web ID: $SG_WEB_ID"

  echo "[2/4] Provisionando Security Group DB (sg-db-tier)..."
  SG_DB_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/security-groups" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{
      \"name\": \"sg-db-tier\",
      \"vpc_id\": \"$VPC_ID\",
      \"account_id\": \"$ACCOUNT_ID\",
      \"rules\": [
        {\"protocol\": \"TCP\", \"start_port\": 5432, \"end_port\": 5432, \"cidr_origin\": \"10.0.0.0/16\"}
      ]
    }")
  SG_DB_ID=$(echo "$SG_DB_RESP" | jq -r '.id // empty')
  if [ -z "$SG_DB_ID" ]; then
    echo "Erro ao criar SG DB: $SG_DB_RESP"
    exit 1
  fi
  echo "      -> SG DB ID: $SG_DB_ID"

  echo "[3/4] Provisionando Instância Virtual Web (web-server-01, flavor: t3.micro)..."
  INST_WEB_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/instances" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{
      \"name\": \"web-server-01\",
      \"flavor\": \"t3.micro\",
      \"vpc_id\": \"$VPC_ID\",
      \"security_group_id\": \"$SG_WEB_ID\",
      \"account_id\": \"$ACCOUNT_ID\"
    }")
  INST_WEB_ID=$(echo "$INST_WEB_RESP" | jq -r '.id // empty')
  INST_WEB_IP=$(echo "$INST_WEB_RESP" | jq -r '.ip_publico // empty')
  echo "      -> Instância Web ID: $INST_WEB_ID (IP Público: $INST_WEB_IP)"

  echo "[4/4] Provisionando Instância Virtual Banco de Dados (db-server-01, flavor: c5.large)..."
  INST_DB_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/instances" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{
      \"name\": \"db-server-01\",
      \"flavor\": \"c5.large\",
      \"vpc_id\": \"$VPC_ID\",
      \"security_group_id\": \"$SG_DB_ID\",
      \"account_id\": \"$ACCOUNT_ID\"
    }")
  INST_DB_ID=$(echo "$INST_DB_RESP" | jq -r '.id // empty')
  INST_DB_IP=$(echo "$INST_DB_RESP" | jq -r '.ip_privado // empty')
  echo "      -> Instância DB ID: $INST_DB_ID (IP Privado: $INST_DB_IP)"

  # Salvar estado local simulado
  cat <<EOF > "$STATE_FILE"
{
  "sg_web_id": "$SG_WEB_ID",
  "sg_db_id": "$SG_DB_ID",
  "inst_web_id": "$INST_WEB_ID",
  "inst_db_id": "$INST_DB_ID"
}
EOF

  echo ""
  echo "Apply complete! Resources: 4 added, 0 changed, 0 destroyed."
  echo ""
  echo "Outputs:"
  echo "gateway_endpoint = \"$GATEWAY_URL\""
  echo "vpc_id = \"$VPC_ID\""
  echo "web_server_id = \"$INST_WEB_ID\" (IP: $INST_WEB_IP)"
  echo "db_server_id = \"$INST_DB_ID\" (IP: $INST_DB_IP)"

elif [ "$ACTION" = "destroy" ]; then
  if [ ! -f "$STATE_FILE" ]; then
    echo "Nenhum arquivo de estado encontrado ($STATE_FILE). Nada a destruir."
    exit 0
  fi

  INST_WEB_ID=$(jq -r '.inst_web_id' "$STATE_FILE")
  INST_DB_ID=$(jq -r '.inst_db_id' "$STATE_FILE")
  SG_WEB_ID=$(jq -r '.sg_web_id' "$STATE_FILE")
  SG_DB_ID=$(jq -r '.sg_db_id' "$STATE_FILE")

  echo "[1/4] Destruindo Instância Web ($INST_WEB_ID)..."
  curl -s -X DELETE "$GATEWAY_URL/v1/instances/$INST_WEB_ID" -H "X-API-Key: $API_KEY"
  
  echo "[2/4] Destruindo Instância DB ($INST_DB_ID)..."
  curl -s -X DELETE "$GATEWAY_URL/v1/instances/$INST_DB_ID" -H "X-API-Key: $API_KEY"

  echo "[3/4] Removendo Security Group Web ($SG_WEB_ID)..."
  curl -s -X DELETE "$GATEWAY_URL/v1/security-groups/$SG_WEB_ID" -H "X-API-Key: $API_KEY"

  echo "[4/4] Removendo Security Group DB ($SG_DB_ID)..."
  curl -s -X DELETE "$GATEWAY_URL/v1/security-groups/$SG_DB_ID" -H "X-API-Key: $API_KEY"

  rm -f "$STATE_FILE" .sg_web_id .sg_db_id .inst_web_id .inst_db_id
  echo ""
  echo "Destroy complete! Resources: 4 destroyed."
fi
