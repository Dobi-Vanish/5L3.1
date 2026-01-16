package main

import (
	"context"
	"log"
	"os"

	"notifier/internal/queue"
	"notifier/internal/storage"
	"notifier/internal/worker"
)

func main() {
	ctx := context.Background()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis:6379"
	}

	store, err := storage.NewRedisStorage(redisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@rabbitmq:5672/"
	}

	queueManager, err := queue.NewManager(amqpURL)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ manager: %v", err)
	}
	defer queueManager.Close()

	scheduler := worker.NewScheduler(store, queueManager)
	scheduler.Start(ctx)
	defer scheduler.Stop()

	processor := worker.NewProcessor(store, queueManager)
	if err := processor.Start(ctx); err != nil {
		log.Fatalf("Failed to start processor: %v", err)
	}
	defer processor.Stop()

	log.Println("Worker started successfully")

	select {}
}
