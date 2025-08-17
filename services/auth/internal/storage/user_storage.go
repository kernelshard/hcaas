package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samims/hcaas/services/auth/internal/model"

	"github.com/google/uuid"
)

type UserStorage interface {
	CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	Ping(ctx context.Context) error
}

type userStorage struct {
	db *pgxpool.Pool
}

func NewUserStorage(dbPool *pgxpool.Pool) UserStorage {
	return &userStorage{db: dbPool}
}

func (s *userStorage) CreateUser(ctx context.Context, email, hashedPass string) (*model.User, error) {
	id := uuid.New().String()
	now := time.Now()
	query := `
		INSERT INTO users (id, email, password, created_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := s.db.Exec(ctx, query, id, email, hashedPass, now)

	if err != nil {
		return nil, err
	}

	return &model.User{
		ID:        id,
		Email:     email,
		Password:  hashedPass,
		CreatedAt: now,
	}, nil
}

func (s *userStorage) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, email, password, created_at
		FROM users
		WHERE email = $1
	`
	row := s.db.QueryRow(ctx, query, email)

	var user model.User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *userStorage) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}
