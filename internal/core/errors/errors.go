package core_errors

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrAlreadyExists      = errors.New("already exists")
)
