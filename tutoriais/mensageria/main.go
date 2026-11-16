package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	log.Printf("[atlas-notification-worker] Iniciando worker de notificações RabbitMQ...")
	worker := NewWorker(rabbitURL)

	go worker.Start()

	// Aguardar sinal de encerramento do sistema (SIGINT, SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Printf("[atlas-notification-worker] Encerrando worker ordenadamente...")
}
