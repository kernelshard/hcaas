package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samims/hcaas/pkg/tracing"
	appErr "github.com/samims/hcaas/services/url/internal/errors"
	"github.com/samims/hcaas/services/url/internal/model"
	"go.opentelemetry.io/otel/attribute"
)

type Storage interface {
	Ping(ctx context.Context) error
	Save(ctx context.Context, url *model.URL) error
	FindAll(ctx context.Context) ([]model.URL, error)
	FindAllByUserID(ctx context.Context, userID string) ([]model.URL, error)
	FindByID(ctx context.Context, id string) (model.URL, error)
	FindByAddress(ctx context.Context, address string) (model.URL, error)
	UpdateStatus(ctx context.Context, id, status string, checkedAt time.Time) error
}

type postgresStorage struct {
	db     *pgxpool.Pool
	tracer *tracing.Tracer
}

func NewPostgresStorage(pool *pgxpool.Pool, tracer *tracing.Tracer) Storage {
	return &postgresStorage{db: pool, tracer: tracer}
}

func (ps *postgresStorage) Ping(ctx context.Context) error {
	return ps.db.Ping(ctx)
}

func (ps *postgresStorage) FindByID(ctx context.Context, id string) (model.URL, error) {
	ctx, span := ps.tracer.StartClientSpan(ctx, "FindByID")
	defer span.End()

	const query = `
		SELECT id, user_id, address, status, checked_at, updated_at, created_at
		FROM urls
		WHERE id = $1
	`

	var url model.URL
	err := ps.db.QueryRow(ctx, query, id).Scan(
		&url.ID, &url.UserID, &url.Address, &url.Status, &url.CheckedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.URL{}, fmt.Errorf("url not found: %w", err)
		}
		span.RecordError(err)
		return model.URL{}, fmt.Errorf("find by id failed: %w", err)
	}

	return url, nil
}

func (ps *postgresStorage) FindAll(ctx context.Context) ([]model.URL, error) {
	ctx, span := ps.tracer.StartClientSpan(ctx, "FindAll")
	defer span.End()

	const query = `
		SELECT id, user_id, address, status, checked_at
		FROM urls
	`

	rows, err := ps.db.Query(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var urls []model.URL

	for rows.Next() {
		var url model.URL
		if err := rows.Scan(&url.ID, &url.UserID, &url.Address, &url.Status, &url.CheckedAt); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		urls = append(urls, url)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("row iteration failed: %w", err)
	}

	return urls, nil
}

func (ps *postgresStorage) FindAllByUserID(ctx context.Context, userID string) ([]model.URL, error) {
	ctx, span := ps.tracer.StartClientSpan(ctx, "FindAllByUserID")
	defer span.End()

	const query = `
		SELECT id, user_id, address, status, checked_at
		from urls
		where user_id = $1
	`
	rows, err := ps.db.Query(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("query failed %w", err)
	}
	defer rows.Close()

	var urls []model.URL

	for rows.Next() {
		var url model.URL
		if err := rows.Scan(&url.ID, &url.UserID, &url.Address, &url.Status, &url.CheckedAt); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan failed %w", err)
		}
		urls = append(urls, url)

	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("row iteration failed %w", err)

	}
	span.SetAttributes(attribute.Int("url.count", len(urls)))
	return urls, nil

}
func (ps *postgresStorage) Save(ctx context.Context, url *model.URL) error {
	ctx, span := ps.tracer.StartClientSpan(ctx, "Save")
	defer span.End()

	const queryStr = `
		INSERT INTO urls(id, user_id, address, status, checked_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	err := ps.db.QueryRow(ctx, queryStr, url.ID, url.UserID, url.Address, url.Status, url.CheckedAt).Scan(&url.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" { // unique_violation
				return appErr.ErrConflict
			}
		}
		span.RecordError(err)
		return fmt.Errorf("failed to save URL: %w", err)
	}

	span.SetAttributes(attribute.String("url.id", url.ID))
	return nil
}

func (ps *postgresStorage) UpdateStatus(ctx context.Context, id string, status string, checkedAt time.Time) error {
	ctx, span := ps.tracer.StartClientSpan(ctx, "UpdateStatus")
	defer span.End()

	const query = `
		UPDATE urls
		SET status = $1, checked_at = $2
		WHERE id = $3
	`

	cmdTags, err := ps.db.Exec(ctx, query, status, checkedAt, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to update status: %w", err)
	}

	if cmdTags.RowsAffected() == 0 {
		err := fmt.Errorf("no record found to update with id %s", id)
		span.RecordError(err)
		return err
	}

	span.SetAttributes(attribute.String("url.id", id))
	span.SetAttributes(attribute.String("url.status", status))
	return nil
}

func (ps *postgresStorage) FindByAddress(ctx context.Context, address string) (model.URL, error) {
	ctx, span := ps.tracer.StartClientSpan(ctx, "FindByAddress")
	defer span.End()

	const query = `
		SELECT id, user_id, address, status, checked_at
		FROM urls
		WHERE address = $1
	`

	var url model.URL
	err := ps.db.QueryRow(ctx, query, address).Scan(
		&url.ID, &url.UserID, &url.Address, &url.Status, &url.CheckedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.URL{}, appErr.ErrNotFound
		}
		span.RecordError(err)
		return model.URL{}, fmt.Errorf("find by address failed: %w", err)
	}

	return url, nil
}
