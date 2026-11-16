output "gateway_endpoint" {
  description = "Ponto de entrada público da nuvem AtlasCloud"
  value       = var.gateway_url
}

output "vpc_id" {
  description = "VPC provisionada"
  value       = var.vpc_id
}

output "provisioned_resources" {
  description = "Relação de recursos gerenciados pelo Terraform"
  value = {
    web_tier = {
      security_group = "sg-web-tier (portas 80, 443)"
      instance       = "web-server-01 (flavor t3.micro, 1 vCPU, 1GB RAM)"
    }
    db_tier = {
      security_group = "sg-db-tier (porta 5432)"
      instance       = "db-server-01 (flavor c5.large, 2 vCPUs, 4GB RAM)"
    }
  }
}
