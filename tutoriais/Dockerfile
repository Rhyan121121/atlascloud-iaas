# Multi-stage Dockerfile para os 6 microsserviços AtlasCloud IaaS em Go
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Compilação dos binários estáticos organizados nas pastas do edital
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/compute ./rest/compute
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/security ./rest/security
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/billing ./rest/billing
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/workers ./mensageria
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/graphql ./graphql
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/gateway ./gateway

# Imagem base de execução leve (Alpine com curl para healthchecks)
FROM alpine:3.20 AS base
RUN apk --no-cache add curl ca-certificates tzdata
WORKDIR /app

# 1. Target Compute Service (Parte 1: REST)
FROM base AS compute
COPY --from=builder /bin/compute /app/compute
EXPOSE 8081
CMD ["/app/compute"]

# 2. Target Security Service (Parte 1: REST)
FROM base AS security
COPY --from=builder /bin/security /app/security
EXPOSE 8082
CMD ["/app/security"]

# 3. Target Billing Service (Parte 1: REST)
FROM base AS billing
COPY --from=builder /bin/billing /app/billing
EXPOSE 8083
CMD ["/app/billing"]

# 4. Target Notification & Audit Workers (Parte 2: Mensageria)
FROM base AS workers
COPY --from=builder /bin/workers /app/workers
CMD ["/app/workers"]

# 5. Target GraphQL BFF Service (Parte 3: GraphQL)
FROM base AS graphql
COPY --from=builder /bin/graphql /app/graphql
EXPOSE 8084
CMD ["/app/graphql"]

# 6. Target API Gateway com Frontend Estático Integrado (Parte 4: Gateway)
FROM base AS gateway
COPY --from=builder /bin/gateway /app/gateway
COPY frontend /app/frontend
EXPOSE 8080
CMD ["/app/gateway"]
