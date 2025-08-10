package errors

import "errors"

var (
	ErrInvalidEmail    = errors.New("invalid email")
	ErrInvalidInput    = errors.New("invalid input")
	ErrConflict        = errors.New("conflict")
	ErrInternal        = errors.New("internal error")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrTokenGeneration = errors.New("token generation failed")
	ErrTooManyAttempts = errors.New("too many login attempts, account locked temporarily")
	ErrNotFound        = errors.New("not found")
)
