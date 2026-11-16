terraform {
  required_version = ">= 1.0.0"
}

# 1. Security Group para Aplicação Web (Portas 80 e 443)
resource "terraform_data" "sg_web" {
  input = {
    name       = "sg-web-tier"
    vpc_id     = var.vpc_id
    account_id = var.account_id
  }

  provisioner "local-exec" {
    command = <<-EOT
      RESPONSE=$(curl -s -X POST "${var.gateway_url}/v1/security-groups" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${var.api_key}" \
        -d '{
          "name": "sg-web-tier",
          "vpc_id": "${var.vpc_id}",
          "account_id": "${var.account_id}",
          "rules": [
            {"protocol": "TCP", "start_port": 80, "end_port": 80, "cidr_origin": "0.0.0.0/0"},
            {"protocol": "TCP", "start_port": 443, "end_port": 443, "cidr_origin": "0.0.0.0/0"}
          ]
        }')
      SG_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
      echo "$SG_ID" > .sg_web_id
      echo "[Terraform] Security Group Web Criado: $SG_ID"
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      if [ -f .sg_web_id ]; then
        SG_ID=$(cat .sg_web_id)
        curl -s -X DELETE "http://localhost:8080/v1/security-groups/$SG_ID" \
          -H "X-API-Key: cli-terraform"
        rm -f .sg_web_id
        echo "[Terraform] Security Group Web Removido: $SG_ID"
      fi
    EOT
  }
}

# 2. Security Group para Banco de Dados (Porta 5432)
resource "terraform_data" "sg_db" {
  input = {
    name       = "sg-db-tier"
    vpc_id     = var.vpc_id
    account_id = var.account_id
  }

  provisioner "local-exec" {
    command = <<-EOT
      RESPONSE=$(curl -s -X POST "${var.gateway_url}/v1/security-groups" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${var.api_key}" \
        -d '{
          "name": "sg-db-tier",
          "vpc_id": "${var.vpc_id}",
          "account_id": "${var.account_id}",
          "rules": [
            {"protocol": "TCP", "start_port": 5432, "end_port": 5432, "cidr_origin": "10.0.0.0/16"}
          ]
        }')
      SG_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
      echo "$SG_ID" > .sg_db_id
      echo "[Terraform] Security Group DB Criado: $SG_ID"
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      if [ -f .sg_db_id ]; then
        SG_ID=$(cat .sg_db_id)
        curl -s -X DELETE "http://localhost:8080/v1/security-groups/$SG_ID" \
          -H "X-API-Key: cli-terraform"
        rm -f .sg_db_id
        echo "[Terraform] Security Group DB Removido: $SG_ID"
      fi
    EOT
  }
}

# 3. Instância Virtual: Servidor Web (t3.micro)
resource "terraform_data" "instance_web" {
  depends_on = [terraform_data.sg_web]

  provisioner "local-exec" {
    command = <<-EOT
      SG_ID=$(cat .sg_web_id)
      RESPONSE=$(curl -s -X POST "${var.gateway_url}/v1/instances" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${var.api_key}" \
        -d "{
          \"name\": \"web-server-01\",
          \"flavor\": \"t3.micro\",
          \"vpc_id\": \"${var.vpc_id}\",
          \"security_group_id\": \"$SG_ID\",
          \"account_id\": \"${var.account_id}\"
        }")
      INST_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
      echo "$INST_ID" > .inst_web_id
      echo "[Terraform] Instância Web Criada: $INST_ID"
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      if [ -f .inst_web_id ]; then
        INST_ID=$(cat .inst_web_id)
        curl -s -X DELETE "http://localhost:8080/v1/instances/$INST_ID" \
          -H "X-API-Key: cli-terraform"
        rm -f .inst_web_id
        echo "[Terraform] Instância Web Removida: $INST_ID"
      fi
    EOT
  }
}

# 4. Instância Virtual: Servidor de Banco de Dados (c5.large)
resource "terraform_data" "instance_db" {
  depends_on = [terraform_data.sg_db]

  provisioner "local-exec" {
    command = <<-EOT
      SG_ID=$(cat .sg_db_id)
      RESPONSE=$(curl -s -X POST "${var.gateway_url}/v1/instances" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${var.api_key}" \
        -d "{
          \"name\": \"db-server-01\",
          \"flavor\": \"c5.large\",
          \"vpc_id\": \"${var.vpc_id}\",
          \"security_group_id\": \"$SG_ID\",
          \"account_id\": \"${var.account_id}\"
        }")
      INST_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
      echo "$INST_ID" > .inst_db_id
      echo "[Terraform] Instância DB Criada: $INST_ID"
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      if [ -f .inst_db_id ]; then
        INST_ID=$(cat .inst_db_id)
        curl -s -X DELETE "http://localhost:8080/v1/instances/$INST_ID" \
          -H "X-API-Key: cli-terraform"
        rm -f .inst_db_id
        echo "[Terraform] Instância DB Removida: $INST_ID"
      fi
    EOT
  }
}
