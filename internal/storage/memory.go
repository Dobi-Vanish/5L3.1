package storage

import (
	"context"
	"sync"
	"time"

	"notifier/internal/models"
)

type MemoryStorage struct {
	mu            sync.RWMutex
	notifications map[string]*models.Notification
}

func (s *MemoryStorage) Create(ctx context.Context, notification *models.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.notifications[notification.ID] = notification
	return nil
}

func (s *MemoryStorage) GetByID(ctx context.Context, id string) (*models.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notification, exists := s.notifications[id]
	if !exists {
		return nil, nil
	}
	return notification, nil
}

func (s *MemoryStorage) Update(ctx context.Context, id string, updateFn func(*models.Notification)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, exists := s.notifications[id]
	if !exists {
		return nil
	}

	updateFn(notification)
	notification.UpdatedAt = time.Now()
	return nil
}

func (s *MemoryStorage) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.notifications, id)
	return nil
}

func (s *MemoryStorage) GetAll(ctx context.Context) ([]*models.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notifications := make([]*models.Notification, 0, len(s.notifications))
	for _, n := range s.notifications {
		notifications = append(notifications, n)
	}
	return notifications, nil
}
