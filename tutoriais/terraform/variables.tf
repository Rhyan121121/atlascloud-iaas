variable "gateway_url" {
  description = "URL base do API Gateway da AtlasCloud"
  type        = string
  default     = "http://localhost:8080"
}

variable "api_key" {
  description = "Chave de autenticação com perfil de automação/Terraform"
  type        = string
  default     = "cli-terraform"
}

variable "account_id" {
  description = "Identificador da conta na nuvem AtlasCloud"
  type        = string
  default     = "acc-fatec-01"
}

variable "vpc_id" {
  description = "Identificador da Virtual Private Cloud"
  type        = string
  default     = "vpc-atlas-prod"
}
