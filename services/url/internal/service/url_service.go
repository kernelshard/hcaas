package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/samims/hcaas/pkg/tracing"
	appErr "github.com/samims/hcaas/services/url/internal/errors"
	"github.com/samims/hcaas/services/url/internal/model"
	"github.com/samims/hcaas/services/url/internal/storage"
)

func getUserIDFromContext(ctx context.Context) (string, error) {
	val := ctx.Value(model.ContextUserIDKey)
	if val == nil {
		return "", appErr.NewInternal("context missing user_id - verify auth middleware is properly configured and executed before service methods")
	}

	userID, ok := val.(string)
	if !ok {
		return "", appErr.NewInternal(fmt.Sprintf(
			"invalid user_id type in context - got %T (%v), expected string",
			val, val))
	}

	if userID == "" {
		return "", appErr.NewInternal("empty user_id in context - verify auth service is returning valid user identifier")
	}

	slog.Debug("Successfully extracted user_id from context",
		"user_id", userID,
		"context_keys", fmt.Sprintf("%+v", ctx))
	return userID, nil
}

type URLService interface {
	GetAll(ctx context.Context) ([]model.URL, error)
	GetByID(ctx context.Context, id string) (*model.URL, error)
	GetAllByUserID(ctx context.Context) ([]model.URL, error)
	Add(ctx context.Context, url model.URL) error
	UpdateStatus(ctx context.Context, id string, status string) error
}

type urlService struct {
	store  storage.Storage
	logger *slog.Logger
	tracer *tracing.Tracer
}

func NewURLService(store storage.Storage, logger *slog.Logger, tracer *tracing.Tracer) URLService {
	l := logger.With("layer", "service", "component", "urlService")
	return &urlService{
		store:  store,
		logger: l,
		tracer: tracer,
	}
}

// GetAllByUserID fetches urls for the user
func (s *urlService) GetAllByUserID(ctx context.Context) ([]model.URL, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "GetAllByUserID", attribute.String("file", "url_service"))
	defer span.End()

	s.logger.Info("GetAllByUserID called")
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		s.tracer.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	// Add the user ID as an attribute to the span.
	span.SetAttributes(attribute.String("user.id", userID))

	userURLs, err := s.store.FindAllByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to fetch URLs",
			slog.String("error", err.Error()),
			slog.String("user_id", userID))
		s.tracer.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, appErr.NewInternal("failed to fetch URLs: %v", err)
	}
	span.SetAttributes(attribute.Int("url.count", len(userURLs)))
	s.logger.Info("GetAllByUserID succeeded", slog.Int("count", len(userURLs)), slog.String("user_id", userID))
	return userURLs, nil
}

func (s *urlService) GetAll(ctx context.Context) ([]model.URL, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "GetAll", attribute.String("file", "url_service"))
	defer span.End()
	s.logger.Info("GetAll called")

	urls, err := s.store.FindAll(ctx)
	if err != nil {
		s.logger.Error("failed to fetch URLs", slog.String("error", err.Error()))
		s.tracer.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, appErr.NewInternal("failed to fetch URLs: %v", err)
	}

	span.SetAttributes(attribute.Int("url.count", len(urls)))
	s.logger.Info("GetAll succeeded", slog.Int("count", len(urls)))
	return urls, nil

}

func (s *urlService) GetByID(ctx context.Context, id string) (*model.URL, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "GetByID", attribute.String("file", "url_service"))
	defer span.End()

	span.SetAttributes(attribute.String("url.id", id))
	s.logger.Info("GetByID called", slog.String("id", id))

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		s.tracer.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", userID))
	span.SetAttributes(attribute.String("url.id", userID))

	url, err := s.store.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, appErr.ErrNotFound) {
			s.logger.Warn("URL not found", slog.String("id", id), slog.String("user_id", userID))
			s.tracer.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			return nil, appErr.NewNotFound(fmt.Sprintf("URL with ID %s not found", id))
		}
		s.logger.Error("failed to fetch URL by ID",
			slog.String("id", id),
			slog.String("user_id", userID),
			slog.String("error", err.Error()))

		s.tracer.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, appErr.NewInternal("failed to fetch URL by ID: %v", err)
	}

	// Verify URL belongs to requesting user
	if url.UserID != userID {
		s.logger.Warn("URL access denied",
			slog.String("id", id),
			slog.String("requested_by", userID),
			slog.String("owned_by", url.UserID))
		ownershipErr := fmt.Errorf("URL access denied %s for user %s", id, userID)
		s.tracer.RecordError(span, ownershipErr)
		return nil, appErr.NewNotFound(fmt.Sprintf("URL with ID %s not found", id))
	}

	s.logger.Info("GetByID succeeded", slog.String("id", id), slog.String("user_id", userID))
	return &url, nil
}

func (s *urlService) Add(ctx context.Context, url model.URL) error {
	ctx, span := s.tracer.StartServerSpan(ctx, "Add", attribute.String("file", "url_service"))
	defer span.End()

	span.SetAttributes(
		attribute.String("url.address", url.Address),
		attribute.String("url.id", url.ID),
	)
	s.logger.Info("Add url called", slog.String("url", url.Address))

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	url.UserID = userID
	span.SetAttributes(attribute.String("user.id", userID))

	// Check if URL address already exists for this user
	existingURL, err := s.store.FindByAddress(ctx, url.Address)
	if err == nil && existingURL.UserID == userID {
		s.logger.Warn("URL address already exists for user",
			slog.String("address", url.Address),
			slog.String("user_id", userID))
		err = appErr.NewConflict("URL address %s already exists", url.Address)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	} else if !errors.Is(err, appErr.ErrNotFound) {
		s.logger.Error("failed to check URL address uniqueness",
			slog.String("address", url.Address),
			slog.Any("error", err))
		err = appErr.NewInternal("failed to check URL address uniqueness: %v", err)

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if url.ID == "" {
		url.ID = uuid.New().String()
	}
	if err := s.store.Save(ctx, &url); err != nil {
		if errors.Is(err, appErr.ErrConflict) {
			s.logger.Warn("URL already exists", slog.String("URL", url.Address))
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return appErr.NewConflict("URL with ID %s already exists", url.ID)
		}

		s.logger.Error("failed to add URL", slog.String("id", url.ID), slog.String("error", err.Error()))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return appErr.NewInternal("failed to add URL: %v", err)
	}

	span.SetAttributes(attribute.String("url.id", url.ID))
	s.logger.Info("Add succeeded", slog.String("id", url.ID), slog.String("user_id", userID))
	return nil
}

// UpdateStatus updates the status of a URL by its ID.
// This is the new, non-user-scoped method for the background checker.
func (s *urlService) UpdateStatus(ctx context.Context, id string, status string) error {
	s.logger.Info("UpdateStatus called by bg task", slog.String("id", id), slog.String("status", status))

	ctx, span := s.tracer.StartServerSpan(ctx, "UpdateStatus", attribute.String("file", "url_service"))
	defer span.End()

	span.SetAttributes(
		attribute.String("url.id", id),
		attribute.String("url.status", status),
	)
	s.logger.Info("UpdateStatus called", slog.String("id", id), slog.String("status", status))

	if err := s.store.UpdateStatus(ctx, id, status, time.Now()); err != nil {
		if errors.Is(err, appErr.ErrNotFound) {
			s.logger.Warn("URL not found for update", slog.String("id", id))
			err := appErr.NewNotFound(fmt.Sprintf("cannot update: URL with ID %s not found", id))
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		s.logger.Error("failed to update status", slog.String("id", id), slog.String("error", err.Error()))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return appErr.NewInternal("failed to update URL status: %v", err)
	}

	s.logger.Info("UpdateStatus succeeded", slog.String("id", id), slog.String("status", status))
	return nil
}
