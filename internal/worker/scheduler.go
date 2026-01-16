package worker

import (
	"context"
	"log"
	"time"

	"github.com/wb-go/wbf/retry"
	"notifier/internal/models"
	"notifier/internal/queue"
	"notifier/internal/storage"
)

type Scheduler struct {
	storage  storage.Storage
	queue    *queue.Manager
	stopChan chan struct{}
}

func NewScheduler(storage storage.Storage, queue *queue.Manager) *Scheduler {
	return &Scheduler{
		storage:  storage,
		queue:    queue,
		stopChan: make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
	log.Println("Scheduler started")
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
	log.Println("Scheduler stopped")
}

func (s *Scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkPendingNotifications(ctx)
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) checkPendingNotifications(ctx context.Context) {
	retryStrategy := retry.Strategy{
		Attempts: 3,
		Delay:    100 * time.Millisecond,
		Backoff:  2,
	}

	var notifications []*models.Notification
	err := retry.DoContext(ctx, retryStrategy, func() error {
		var getErr error
		notifications, getErr = s.storage.GetAll(ctx)
		return getErr
	})

	if err != nil {
		log.Printf("Error getting notifications: %v", err)
		return
	}

	now := time.Now()
	for _, notification := range notifications {
		if notification.Status == models.StatusPending &&
			notification.SendAt.Before(now) &&
			notification.SendAt.After(now.Add(-24*time.Hour)) {

			publishErr := retry.DoContext(ctx, retryStrategy, func() error {
				return s.queue.PublishImmediate(ctx, notification)
			})

			if publishErr != nil {
				log.Printf("Failed to publish notification %s: %v", notification.ID, publishErr)
			} else {
				s.storage.Update(ctx, notification.ID, func(n *models.Notification) {
					n.Status = models.StatusRetrying
					n.UpdatedAt = time.Now()
				})
			}
		}
	}
}
