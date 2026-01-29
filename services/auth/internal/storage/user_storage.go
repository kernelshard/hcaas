package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samims/otelkit"
	"go.opentelemetry.io/otel/attribute"

	"github.com/kernelshard/hcaas/services/auth/internal/model"

	"github.com/google/uuid"
)

// UserStorage interface for user storage
type UserStorage interface {
	CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	Ping(ctx context.Context) error
}

// userStorage struct for user storage
type userStorage struct {
	db     *pgxpool.Pool
	tracer *otelkit.Tracer
}

// NewUserStorage creates a new UserStorage
func NewUserStorage(dbPool *pgxpool.Pool, tracer *otelkit.Tracer) UserStorage {
	return &userStorage{db: dbPool, tracer: tracer}
}

// CreateUser creates a new user
func (s *userStorage) CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error) {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.CreateUser")
	defer span.End()

	id := uuid.New().String()
	now := time.Now()
	query := `
		INSERT INTO users (id, email, password, created_at)
		VALUES ($1, $2, $3, $4)
	`
	span.SetAttributes(
		attribute.String("user.email", email),
		attribute.String("operation", "create_user"),
		attribute.String("storage.component", "user_storage"),
	)

	_, err := s.db.Exec(ctx, query, id, email, hashedPass, now)

	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.type", "database_insert_error"))
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", id))
	span.AddEvent("user.created.success")
	return &model.User{
		ID:        id,
		Email:     email,
		Password:  hashedPass,
		CreatedAt: now,
	}, nil
}

// GetUserByEmail gets a user by email
func (s *userStorage) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.GetUserByEmail")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.email", email),
		attribute.String("operation", "get_user_by_email"),
		attribute.String("storage.component", "user_storage"),
	)

	query := `
		SELECT id, email, password, created_at
		FROM users
		WHERE email = $1
	`
	row := s.db.QueryRow(ctx, query, email)

	var user model.User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.type", "database_query_error"))
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", user.ID))
	span.AddEvent("user.retrieved.success")
	return &user, nil
}

// Ping checks if the database is connected
func (s *userStorage) Ping(ctx context.Context) error {
	ctx, span := s.tracer.StartClientSpan(ctx, "userStorage.Ping")
	defer span.End()

	span.SetAttributes(
		attribute.String("operation", "database_ping"),
		attribute.String("storage.component", "user_storage"),
	)

	err := s.db.Ping(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.type", "database_connection_error"))
		return err
	}
	span.AddEvent("database.connection.healthy")
	return nil
}
