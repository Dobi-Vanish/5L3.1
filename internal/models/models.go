package models

import (
	"time"
)

type NotificationStatus string

const (
	StatusPending   NotificationStatus = "pending"
	StatusSent      NotificationStatus = "sent"
	StatusFailed    NotificationStatus = "failed"
	StatusCancelled NotificationStatus = "cancelled"
	StatusRetrying  NotificationStatus = "retrying"
)

type Notification struct {
	ID         string             `json:"id"`
	Message    string             `json:"message"`
	SendAt     time.Time          `json:"send_at"`
	Status     NotificationStatus `json:"status"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	Attempts   int                `json:"attempts"`
	MaxRetries int                `json:"max_retries"`
	NextRetry  *time.Time         `json:"next_retry,omitempty"`
}

type CreateNotificationRequest struct {
	Message    string    `json:"message"`
	SendAt     time.Time `json:"send_at"`
	MaxRetries int       `json:"max_retries,omitempty"`
}
