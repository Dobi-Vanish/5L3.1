package storage

import (
	"context"

	"notifier/internal/models"
)

type Storage interface {
	Create(ctx context.Context, notification *models.Notification) error
	GetByID(ctx context.Context, id string) (*models.Notification, error)
	Update(ctx context.Context, id string, updateFn func(*models.Notification)) error
	Delete(ctx context.Context, id string) error
	GetAll(ctx context.Context) ([]*models.Notification, error)
}
