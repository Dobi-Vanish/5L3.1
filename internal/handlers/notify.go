package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"notifier/internal/models"
	"notifier/internal/queue"
	"notifier/internal/storage"
)

type NotifyHandler struct {
	storage storage.Storage
	queue   *queue.Manager
}

func NewNotifyHandler(storage storage.Storage, queue *queue.Manager) *NotifyHandler {
	return &NotifyHandler{
		storage: storage,
		queue:   queue,
	}
}

func (h *NotifyHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req models.CreateNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	maxRetries := req.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	notification := &models.Notification{
		ID:         generateID(),
		Message:    req.Message,
		SendAt:     req.SendAt,
		Status:     models.StatusPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Attempts:   0,
		MaxRetries: maxRetries,
		NextRetry:  nil,
	}

	if err := h.storage.Create(ctx, notification); err != nil {
		http.Error(w, "Failed to create notification", http.StatusInternalServerError)
		return
	}

	if err := h.queue.PublishDelayed(ctx, notification); err != nil {
		http.Error(w, "Failed to schedule notification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(notification)
}

func (h *NotifyHandler) GetNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	notification, err := h.storage.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get notification", http.StatusInternalServerError)
		return
	}

	if notification == nil {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notification)
}

func (h *NotifyHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	notification, err := h.storage.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get notification", http.StatusInternalServerError)
		return
	}

	if notification == nil {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	if err := h.storage.Update(ctx, id, func(n *models.Notification) {
		n.Status = models.StatusCancelled
	}); err != nil {
		http.Error(w, "Failed to cancel notification", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *NotifyHandler) GetAllNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	notifications, err := h.storage.GetAll(ctx)
	if err != nil {
		http.Error(w, "Failed to get notifications", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(6)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
