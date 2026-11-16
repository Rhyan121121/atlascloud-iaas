#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
cd "$DIR"

PID_DIR="$DIR/.pids"
LOG_DIR="$DIR/.logs"

mkdir -p "$PID_DIR" "$LOG_DIR"

ensure_rabbitmq() {
  if ! curl -s "http://guest:guest@localhost:15672/api/overview" >/dev/null 2>&1; then
    echo "Iniciando container do RabbitMQ 4 (docker compose up -d rabbitmq)..."
    docker compose up -d rabbitmq >/dev/null 2>&1 || docker start atlas-rabbitmq >/dev/null 2>&1 || true
    echo -n "Aguardando RabbitMQ ficar pronto na porta 5672/15672"
    for _ in $(seq 1 25); do
      if curl -s "http://guest:guest@localhost:15672/api/overview" >/dev/null 2>&1; then
        echo " ✔ Pronto!"
        break
      fi
      echo -n "."
      sleep 1
    done
    echo ""
  fi
}

build_all() {
  echo "Compilando os 6 microsserviços em Go..."
  mkdir -p bin
  go build -o bin/atlas-compute ./rest/compute
  go build -o bin/atlas-security ./rest/security
  go build -o bin/atlas-billing ./rest/billing
  go build -o bin/atlas-workers ./mensageria
  go build -o bin/atlas-graphql ./graphql
  go build -o bin/atlas-gateway ./gateway
  echo "✔ Compilação concluída com sucesso."
}

start_services() {
  stop_services >/dev/null 2>&1 || true
  ensure_rabbitmq
  build_all

  echo "Iniciando microsserviços AtlasCloud em background..."

  # 1. Compute (127.0.0.1:8081)
  PORT=8081 RABBITMQ_URL="amqp://guest:guest@127.0.0.1:5672/" \
    nohup ./bin/atlas-compute > "$LOG_DIR/compute.log" 2>&1 &
  echo $! > "$PID_DIR/compute.pid"

  # 2. Security (127.0.0.1:8082)
  PORT=8082 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" \
    nohup ./bin/atlas-security > "$LOG_DIR/security.log" 2>&1 &
  echo $! > "$PID_DIR/security.pid"

  # 3. Billing (127.0.0.1:8083)
  PORT=8083 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" RABBITMQ_URL="amqp://guest:guest@127.0.0.1:5672/" \
    nohup ./bin/atlas-billing > "$LOG_DIR/billing.log" 2>&1 &
  echo $! > "$PID_DIR/billing.pid"

  # 4. Workers (AMQP Consumers: Notificações + Auditoria)
  RABBITMQ_URL="amqp://guest:guest@127.0.0.1:5672/" \
    nohup ./bin/atlas-workers > "$LOG_DIR/workers.log" 2>&1 &
  echo $! > "$PID_DIR/workers.pid"

  # 5. GraphQL (127.0.0.1:8084)
  PORT=8084 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" SECURITY_SERVICE_URL="http://127.0.0.1:8082" BILLING_SERVICE_URL="http://127.0.0.1:8083" \
    nohup ./bin/atlas-graphql > "$LOG_DIR/graphql.log" 2>&1 &
  echo $! > "$PID_DIR/graphql.pid"

  # 6. Gateway (0.0.0.0:8080 - única porta pública)
  PORT=8080 UPSTREAM_TIMEOUT_MS=2000 COMPUTE_SERVICE_URL="http://127.0.0.1:8081" SECURITY_SERVICE_URL="http://127.0.0.1:8082" BILLING_SERVICE_URL="http://127.0.0.1:8083" GRAPHQL_SERVICE_URL="http://127.0.0.1:8084" STATIC_DIR="./frontend" \
    nohup ./bin/atlas-gateway > "$LOG_DIR/gateway.log" 2>&1 &
  echo $! > "$PID_DIR/gateway.pid"

  sleep 2
  echo ""
  status_services
  echo ""
  echo "✔ Todos os serviços iniciados! Ponto único de entrada: http://localhost:8080"
}

stop_services() {
  echo "Encerrando processos locais..."
  for pid_file in "$PID_DIR"/*.pid; do
    if [ -f "$pid_file" ]; then
      PID=$(cat "$pid_file")
      if kill -0 "$PID" >/dev/null 2>&1; then
        kill "$PID" || true
      fi
      rm -f "$pid_file"
    fi
  done
  echo "✔ Processos encerrados."
}

status_services() {
  echo "Status dos Serviços AtlasCloud:"
  printf "%-25s %-10s %-10s %-22s\n" "SERVIÇO" "PID" "STATUS" "ENDEREÇO / VISIBILIDADE"
  echo "----------------------------------------------------------------------"

  check_p() {
    NAME=$1
    ADDR=$2
    FILE="$PID_DIR/$NAME.pid"
    if [ -f "$FILE" ]; then
      PID=$(cat "$FILE")
      if kill -0 "$PID" >/dev/null 2>&1; then
        printf "%-25s %-10s %-10s %-22s\n" "atlas-$NAME" "$PID" "RUNNING" "$ADDR"
        return
      fi
    fi
    printf "%-25s %-10s %-10s %-22s\n" "atlas-$NAME" "-" "STOPPED" "$ADDR"
  }

  check_p "gateway"  "0.0.0.0:8080 (PÚBLICO)"
  check_p "compute"  "127.0.0.1:8081 (INTERNO)"
  check_p "security" "127.0.0.1:8082 (INTERNO)"
  check_p "billing"  "127.0.0.1:8083 (INTERNO)"
  check_p "graphql"  "127.0.0.1:8084 (INTERNO)"
  check_p "workers"  "AMQP 0-9-1 (2 CONSUMIDORES)"
}

case "$1" in
  start)
    start_services
    ;;
  stop)
    stop_services
    ;;
  restart)
    stop_services
    start_services
    ;;
  status)
    status_services
    ;;
  build)
    build_all
    ;;
  test)
    ensure_rabbitmq
    if ! curl -s "http://localhost:8080/healthz" >/dev/null 2>&1; then
      start_services
    fi
    bash "$DIR/testes/executar_todas_as_cenas.sh"
    ;;
  *)
    echo "Uso: ./dev.sh {start|stop|restart|status|build|test}"
    exit 1
    ;;
esac
