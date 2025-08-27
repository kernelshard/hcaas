package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samims/otelkit"
	"go.opentelemetry.io/otel/attribute"

	"github.com/samims/hcaas/services/auth/internal/model"

	"github.com/google/uuid"
)

type UserStorage interface {
	CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	Ping(ctx context.Context) error
}

type userStorage struct {
	db     *pgxpool.Pool
	tracer *otelkit.Tracer
}

func NewUserStorage(dbPool *pgxpool.Pool, tracer *otelkit.Tracer) UserStorage {
	return &userStorage{db: dbPool, tracer: tracer}
}

func (s *userStorage) CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error) {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.CreateUser")
	defer span.End()

	id := uuid.New().String()
	now := time.Now()
	query := `
		INSERT INTO users (id, email, password, created_at)
		VALUES ($1, $2, $3, $4)
	`
	span.SetAttributes(attribute.String("user.email", email))

	_, err := s.db.Exec(ctx, query, id, email, hashedPass, now)

	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", id))
	return &model.User{
		ID:        id,
		Email:     email,
		Password:  hashedPass,
		CreatedAt: now,
	}, nil
}

func (s *userStorage) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.GetUserByEmail")
	defer span.End()

	span.SetAttributes(attribute.String("user.email", email))

	query := `
		SELECT id, email, password, created_at
		FROM users
		WHERE email = $1
	`
	row := s.db.QueryRow(ctx, query, email)

	var user model.User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", user.ID))
	return &user, nil
}

func (s *userStorage) Ping(ctx context.Context) error {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.Ping")
	defer span.End()

	err := s.db.Ping(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return err
}
