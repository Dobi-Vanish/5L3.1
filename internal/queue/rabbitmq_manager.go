package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/wb-go/wbf/rabbitmq"
	"github.com/wb-go/wbf/retry"
	"notifier/internal/models"
)

type Manager struct {
	client    *rabbitmq.RabbitClient
	publisher *rabbitmq.Publisher
	consumer  *rabbitmq.Consumer
}

func NewManager(url string) (*Manager, error) {
	config := rabbitmq.ClientConfig{
		URL:       url,
		Heartbeat: 10 * time.Second,
		ReconnectStrat: retry.Strategy{
			Attempts: 10,
			Delay:    2 * time.Second,
			Backoff:  2,
		},
		ProducingStrat: retry.Strategy{
			Attempts: 3,
			Delay:    100 * time.Millisecond,
			Backoff:  2,
		},
		ConsumingStrat: retry.Strategy{
			Attempts: 3,
			Delay:    100 * time.Millisecond,
			Backoff:  2,
		},
	}

	client, err := rabbitmq.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create RabbitMQ client: %w", err)
	}

	if err := setupExchangesAndQueues(client); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to setup exchanges and queues: %w", err)
	}

	publisher := rabbitmq.NewPublisher(client, "notifications", "application/json")

	log.Println("RabbitMQ manager initialized successfully")
	return &Manager{
		client:    client,
		publisher: publisher,
	}, nil
}

func setupExchangesAndQueues(client *rabbitmq.RabbitClient) error {
	err := client.DeclareExchange("notifications", "direct", true, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	delayQueueArgs := map[string]interface{}{
		"x-dead-letter-exchange":    "notifications",
		"x-dead-letter-routing-key": "ready",
		"x-message-ttl":             60000,
	}

	err = client.DeclareQueue(
		"notifications.delayed",
		"notifications",
		"delayed",
		true,
		false,
		true,
		delayQueueArgs,
	)
	if err != nil {
		return fmt.Errorf("failed to declare delayed queue: %w", err)
	}

	err = client.DeclareQueue(
		"notifications.ready",
		"notifications",
		"ready",
		true,
		false,
		true,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare ready queue: %w", err)
	}

	return nil
}

func (m *Manager) PublishDelayed(ctx context.Context, notification *models.Notification) error {
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	delay := calculateDelay(notification.SendAt)

	if delay > 60*time.Second {
		log.Printf("Notification %s has long delay %v, will be handled by scheduler",
			notification.ID, delay)
		return nil
	}

	var routingKey string
	var opts []rabbitmq.PublishOption

	if delay <= 0 {
		routingKey = "ready"
	} else {
		routingKey = "delayed"
		opts = append(opts, rabbitmq.WithExpiration(delay))
	}

	err = m.publisher.Publish(ctx, body, routingKey, opts...)
	if err != nil {
		return fmt.Errorf("failed to publish notification: %w", err)
	}

	log.Printf("Published notification %s with routing key %s, delay %v",
		notification.ID, routingKey, delay)
	return nil
}

func (m *Manager) PublishImmediate(ctx context.Context, notification *models.Notification) error {
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	err = m.publisher.Publish(ctx, body, "ready")
	if err != nil {
		return fmt.Errorf("failed to publish notification: %w", err)
	}

	log.Printf("Published immediate notification %s", notification.ID)
	return nil
}

func (m *Manager) StartConsumer(ctx context.Context, handler rabbitmq.MessageHandler) error {
	config := rabbitmq.ConsumerConfig{
		Queue:         "notifications.ready",
		ConsumerTag:   "notifications-consumer",
		AutoAck:       false,
		Workers:       3,
		PrefetchCount: 10,
		Ask: rabbitmq.AskConfig{
			Multiple: false,
		},
		Nack: rabbitmq.NackConfig{
			Multiple: false,
			Requeue:  true,
		},
		Args: nil,
	}

	m.consumer = rabbitmq.NewConsumer(m.client, config, handler)

	go func() {
		if err := m.consumer.Start(ctx); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	log.Println("Consumer started successfully")
	return nil
}

func (m *Manager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

func calculateDelay(sendAt time.Time) time.Duration {
	now := time.Now()
	if sendAt.Before(now) {
		return 0
	}
	return sendAt.Sub(now)
}
