package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	wbfredis "github.com/wb-go/wbf/redis"
	wbfretry "github.com/wb-go/wbf/retry"
	"notifier/internal/models"
)

type RedisStorage struct {
	client *redis.Client
}

func NewRedisStorage(addr string) (*RedisStorage, error) {
	wbfClient := wbfredis.New(addr, "", 0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	retryStrategy := wbfretry.Strategy{
		Attempts: 5,
		Delay:    1 * time.Second,
		Backoff:  2,
	}

	var pingErr error
	err := wbfretry.DoContext(ctx, retryStrategy, func() error {
		pingErr = wbfClient.Ping(ctx)
		return pingErr
	})

	if err != nil || pingErr != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Successfully connected to Redis using wbf/redis client")

	return &RedisStorage{
		client: wbfClient.Client,
	}, nil
}

func (s *RedisStorage) Create(ctx context.Context, notification *models.Notification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	retryStrategy := wbfretry.Strategy{
		Attempts: 3,
		Delay:    100 * time.Millisecond,
		Backoff:  2,
	}

	err = wbfretry.DoContext(ctx, retryStrategy, func() error {
		return s.client.Set(ctx, "notification:"+notification.ID, data, 0).Err()
	})
	if err != nil {
		return fmt.Errorf("failed to store notification: %w", err)
	}

	err = wbfretry.DoContext(ctx, retryStrategy, func() error {
		return s.client.SAdd(ctx, "notifications:all", notification.ID).Err()
	})
	if err != nil {
		return fmt.Errorf("failed to add to notifications set: %w", err)
	}

	if notification.Status == models.StatusPending || notification.Status == models.StatusRetrying {
		var sendTime time.Time
		if notification.Status == models.StatusRetrying && notification.NextRetry != nil {
			sendTime = *notification.NextRetry
		} else {
			sendTime = notification.SendAt
		}

		err = wbfretry.DoContext(ctx, retryStrategy, func() error {
			return s.client.ZAdd(ctx, "notifications:pending", &redis.Z{
				Score:  float64(sendTime.Unix()),
				Member: notification.ID,
			}).Err()
		})
		if err != nil {
			return fmt.Errorf("failed to add to pending notifications: %w", err)
		}
	}

	return nil
}

func (s *RedisStorage) GetByID(ctx context.Context, id string) (*models.Notification, error) {
	retryStrategy := wbfretry.Strategy{
		Attempts: 3,
		Delay:    100 * time.Millisecond,
		Backoff:  2,
	}

	var data []byte

	retryErr := wbfretry.DoContext(ctx, retryStrategy, func() error {
		result, getErr := s.client.Get(ctx, "notification:"+id).Bytes()
		if getErr != nil && getErr != redis.Nil {
			return getErr
		}
		data = result
		return nil
	})

	if retryErr != nil && retryErr != redis.Nil {
		return nil, fmt.Errorf("failed to get notification: %w", retryErr)
	}

	if data == nil {
		return nil, nil
	}

	var notification models.Notification
	if err := json.Unmarshal(data, &notification); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	return &notification, nil
}

func (s *RedisStorage) Update(ctx context.Context, id string, updateFn func(*models.Notification)) error {
	notification, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found")
	}

	oldStatus := notification.Status
	updateFn(notification)
	notification.UpdatedAt = time.Now()

	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	retryStrategy := wbfretry.Strategy{
		Attempts: 3,
		Delay:    100 * time.Millisecond,
		Backoff:  2,
	}

	err = wbfretry.DoContext(ctx, retryStrategy, func() error {
		return s.client.Set(ctx, "notification:"+id, data, 0).Err()
	})
	if err != nil {
		return fmt.Errorf("failed to update notification: %w", err)
	}

	if oldStatus != notification.Status {
		s.client.ZRem(ctx, "notifications:pending", id)

		if notification.Status == models.StatusPending || notification.Status == models.StatusRetrying {
			var sendTime time.Time
			if notification.Status == models.StatusRetrying && notification.NextRetry != nil {
				sendTime = *notification.NextRetry
			} else {
				sendTime = notification.SendAt
			}

			s.client.ZAdd(ctx, "notifications:pending", &redis.Z{
				Score:  float64(sendTime.Unix()),
				Member: id,
			})
		}
	}

	return nil
}

func (s *RedisStorage) Delete(ctx context.Context, id string) error {
	retryStrategy := wbfretry.Strategy{
		Attempts: 3,
		Delay:    100 * time.Millisecond,
		Backoff:  2,
	}

	err := wbfretry.DoContext(ctx, retryStrategy, func() error {
		return s.client.Del(ctx, "notification:"+id).Err()
	})
	if err != nil {
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	s.client.SRem(ctx, "notifications:all", id)
	s.client.ZRem(ctx, "notifications:pending", id)

	return nil
}

func (s *RedisStorage) GetAll(ctx context.Context) ([]*models.Notification, error) {
	ids, err := s.client.SMembers(ctx, "notifications:all").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get notification IDs: %w", err)
	}

	var notifications []*models.Notification
	for _, id := range ids {
		notification, err := s.GetByID(ctx, id)
		if err != nil {
			log.Printf("Error getting notification %s: %v", id, err)
			continue
		}
		if notification != nil {
			notifications = append(notifications, notification)
		}
	}

	return notifications, nil
}

func (s *RedisStorage) GetPendingNotifications(ctx context.Context) ([]*models.Notification, error) {
	now := time.Now().Unix()

	ids, err := s.client.ZRangeByScore(ctx, "notifications:pending", &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending notifications: %w", err)
	}

	var notifications []*models.Notification
	for _, id := range ids {
		notification, err := s.GetByID(ctx, id)
		if err != nil {
			log.Printf("Error getting notification %s: %v", id, err)
			continue
		}
		if notification != nil {
			notifications = append(notifications, notification)
		}
	}

	return notifications, nil
}
