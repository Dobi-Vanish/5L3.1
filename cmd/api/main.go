package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"notifier/internal/handlers"
	"notifier/internal/queue"
	"notifier/internal/storage"
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

	handler := handlers.NewNotifyHandler(store, queueManager)

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Timeout(60 * time.Second))

	fs := http.FileServer(http.Dir("./ui"))
	r.Handle("/*", fs)

	r.Route("/api/notify", func(r chi.Router) {
		r.Post("/", handler.CreateNotification)
		r.Get("/", handler.GetAllNotifications)
		r.Get("/{id}", handler.GetNotification)
		r.Delete("/{id}", handler.DeleteNotification)
	})

	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	r.Get("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		notifications, err := store.GetAll(ctx)
		if err != nil {
			http.Error(w, "Failed to get metrics", http.StatusInternalServerError)
			return
		}

		stats := map[string]int{
			"total":     len(notifications),
			"pending":   0,
			"sent":      0,
			"failed":    0,
			"cancelled": 0,
			"retrying":  0,
		}

		for _, n := range notifications {
			stats[string(n.Status)]++
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	port := ":8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	server := &http.Server{
		Addr:    port,
		Handler: r,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("API server starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")
}
