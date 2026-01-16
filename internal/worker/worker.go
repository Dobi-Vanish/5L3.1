package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rabbitmq/amqp091-go"
	"log"
	"math"
	"time"

	"github.com/wb-go/wbf/retry"
	"notifier/internal/models"
	"notifier/internal/queue"
	"notifier/internal/storage"
)

type Processor struct {
	storage  storage.Storage
	queue    *queue.Manager
	stopChan chan struct{}
}

func NewProcessor(storage storage.Storage, queue *queue.Manager) *Processor {
	return &Processor{
		storage:  storage,
		queue:    queue,
		stopChan: make(chan struct{}),
	}
}

func (p *Processor) Start(ctx context.Context) error {
	err := p.queue.StartConsumer(ctx, p.handleMessage)
	if err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	log.Println("Processor started successfully")
	return nil
}

func (p *Processor) Stop() {
	close(p.stopChan)
	log.Println("Processor stopped")
}

func (p *Processor) handleMessage(ctx context.Context, delivery amqp091.Delivery) error {
	var notification models.Notification
	if err := json.Unmarshal(delivery.Body, &notification); err != nil {
		log.Printf("Failed to unmarshal notification: %v", err)
		return err
	}

	log.Printf("Processing notification %s scheduled for %v",
		notification.ID, notification.SendAt)

	if notification.SendAt.After(time.Now()) {
		log.Printf("Notification %s is not ready yet, will be requeued", notification.ID)
		return fmt.Errorf("notification not ready")
	}

	storedNotification, err := p.storage.GetByID(ctx, notification.ID)
	if err != nil {
		log.Printf("Error getting notification %s: %v", notification.ID, err)
		return err
	}

	if storedNotification == nil {
		log.Printf("Notification %s not found", notification.ID)
		return nil
	}

	if storedNotification.Status == models.StatusCancelled {
		log.Printf("Notification %s was cancelled", notification.ID)
		return nil
	}

	if storedNotification.Attempts > 0 {
		err = p.storage.Update(ctx, notification.ID, func(n *models.Notification) {
			n.Status = models.StatusRetrying
		})
		if err != nil {
			log.Printf("Failed to update notification %s: %v", notification.ID, err)
		}
	}

	retryStrategy := retry.Strategy{
		Attempts: storedNotification.MaxRetries - storedNotification.Attempts,
		Delay:    1 * time.Second,
		Backoff:  2,
	}

	var success bool
	retryErr := retry.DoContext(ctx, retryStrategy, func() error {
		success = p.sendNotification(storedNotification)
		if !success {
			return &SendError{Message: "Failed to send notification"}
		}
		return nil
	})

	err = p.storage.Update(ctx, notification.ID, func(n *models.Notification) {
		n.Attempts++

		if success {
			n.Status = models.StatusSent
			n.NextRetry = nil
			log.Printf("Notification %s sent successfully", notification.ID)
		} else {
			if n.Attempts >= n.MaxRetries || retryErr != nil {
				n.Status = models.StatusFailed
				n.NextRetry = nil
				log.Printf("Notification %s failed after %d attempts: %v",
					notification.ID, n.Attempts, retryErr)
			} else {
				delay := time.Duration(math.Pow(2, float64(n.Attempts))) * time.Second
				nextRetry := time.Now().Add(delay)
				n.NextRetry = &nextRetry
				n.Status = models.StatusRetrying

				n.SendAt = nextRetry
				go func() {
					if err := p.queue.PublishDelayed(ctx, n); err != nil {
						log.Printf("Failed to schedule retry for notification %s: %v",
							notification.ID, err)
					}
				}()

				log.Printf("Notification %s failed, will retry in %v",
					notification.ID, delay)
			}
		}
	})

	if err != nil {
		log.Printf("Failed to update notification %s: %v", notification.ID, err)
		return err
	}

	return nil
}

func (p *Processor) sendNotification(notification *models.Notification) bool {

	if notification.Attempts < 2 {
		return true
	}
	return false
}

type SendError struct {
	Message string
}

func (e *SendError) Error() string {
	return e.Message
}
