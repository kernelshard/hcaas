package service

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	appErr "github.com/kernelshard/hcaas/services/auth/internal/errors"
	"github.com/kernelshard/hcaas/services/auth/internal/model"
	"github.com/kernelshard/hcaas/services/auth/internal/storage"

	"github.com/stretchr/testify/mock"
)

// Test_authService_Register tests the Register method of the authService.
// Table Driven Test Pattern used
func Test_authService_Register(t *testing.T) {
	mockLogger := slog.Default()

	type fields struct {
		store    storage.UserStorage
		logger   *slog.Logger
		tokenSvc TokenService
	}
	type args struct {
		ctx      context.Context
		email    string
		password string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *model.User
		wantErr bool
	}{
		{
			name: "successful registration",
			fields: fields{
				store: func() storage.UserStorage {
					sut := storage.NewMockUserStorage(t)
					sut.On("CreateUser", context.Background(), "test1@example.com", mock.Anything).
						Return(&model.User{
							Email: "test1@example.com",
						}, nil)
					return sut
				}(),
				logger:   mockLogger,
				tokenSvc: nil,
			},
			args: args{
				ctx:      context.Background(),
				email:    "test1@example.com",
				password: "Password@123",
			},
			want: &model.User{
				Email: "test1@example.com",
			},
			wantErr: false,
		},
		{
			name: "failed registration existing user",
			fields: fields{
				store: func() storage.UserStorage {
					sut := storage.NewMockUserStorage(t)
					sut.On("CreateUser", context.Background(), "test1@example.com", mock.Anything).
						Return(nil, appErr.ErrConflict)
					return sut
				}(),
				logger:   mockLogger,
				tokenSvc: nil,
			},
			args: args{
				ctx:      context.Background(),
				email:    "test1@example.com",
				password: "Password@123",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed registration empty email",
			fields: fields{
				store: func() storage.UserStorage {
					sut := storage.NewMockUserStorage(t)
					return sut
				}(),
				logger:   mockLogger,
				tokenSvc: nil,
			},
			args: args{
				ctx:      context.Background(),
				email:    "",
				password: "Password@123",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed registration invalid email",
			fields: fields{
				store: func() storage.UserStorage {
					sut := storage.NewMockUserStorage(t)
					return sut
				}(),
				logger:   mockLogger,
				tokenSvc: nil,
			},
			args: args{
				ctx:      context.Background(),
				email:    "test1.example.com",
				password: "Password@123",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed registration no password",
			fields: fields{
				store: func() storage.UserStorage {
					sut := storage.NewMockUserStorage(t)
					return sut
				}(),
				logger:   mockLogger,
				tokenSvc: nil,
			},
			args: args{
				ctx:      context.Background(),
				email:    "test1@example.com",
				password: "",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &authService{
				store:    tt.fields.store,
				logger:   tt.fields.logger,
				tokenSvc: tt.fields.tokenSvc,
			}
			got, err := s.Register(tt.args.ctx, tt.args.email, tt.args.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("authService.Register() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("authService.Register() = %v, want %v", got, tt.want)
			}
		})
	}
}
