package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kernelshard/hcaas/services/notification/internal/model"
)

type postgresStorage struct {
	db *sqlx.DB
}

// NewPostgresStorage creates connection pool
func NewPostgresStorage(db *sqlx.DB) (NotificationStorage, error) {
	return &postgresStorage{db: db}, nil
}

// Save inserts a new notification with status pending
func (s *postgresStorage) Save(ctx context.Context, notif *model.Notification) error {
	if notif == nil {
		return fmt.Errorf("notification cannot be nil")
	}
	query := `INSERT INTO notifications
		(url_id, type, message, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at, updated_at`

	row := s.db.QueryRowxContext(
		ctx, query, notif.UrlId, notif.Type, notif.Message, notif.Status, notif.CreatedAt, notif.UpdatedAt)
	if err := row.Scan(&notif.ID, &notif.CreatedAt, &notif.UpdatedAt); err != nil {
		return err
	}
	return nil
}

// GetPending returns notifications with status pending
func (s *postgresStorage) GetPending(ctx context.Context) ([]model.Notification, error) {
	var notifs []model.Notification
	query := `SELECT * FROM notifications where status = $1`
	err := s.db.SelectContext(ctx, &notifs, query, model.StatusPending)
	if err != nil {
		return nil, err
	}
	return notifs, nil
}

// UpdateStatus updates notification status and timestamp
func (s *postgresStorage) UpdateStatus(ctx context.Context, id int, status string) error {
	query := "UPDATE notifications SET status=$1, updated_at=$2 where id=$3"
	res, err := s.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return err
	}
	// Always check the error returned
	//by RowsAffected.
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("no rows were updated, check if the ID is correct")
	}
	return nil
}

func (s *postgresStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
