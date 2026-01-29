package store

import (
	"context"

	"github.com/kernelshard/hcaas/services/notification/internal/model"
)

// NotificationStorage defines DB operations for notifications
// Allows persisting notification requests for async processing
type NotificationStorage interface {
	Save(ctx context.Context, n *model.Notification) error
	GetPending(ctx context.Context) ([]model.Notification, error)
	UpdateStatus(ctx context.Context, id int, status string) error
	Ping(ctx context.Context) error
}
