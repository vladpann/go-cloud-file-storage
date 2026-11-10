package core_http_v1_users

import (
	"time"

	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/internal/domain"
)

// CreateUserRequest represents the sign-up request
type CreateUserRequest struct {
	Username       string `json:"username" validate:"required"`
	Password       string `json:"password" validate:"required,min=5,max=72"`
	RepeatPassword string `json:"repeat_password" validate:"required"`
}

// CreateUserResponse represents the sign-up response
type CreateUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	SessionID uuid.UUID `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SignInRequest represents the sign-in request
type SignInRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// SignInResponse represents the sign-in response
type SignInResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// createUserResponseFromDomain converts a domain user into the sign-up response DTO
func createUserResponseFromDomain(user domain.User, session domain.Session) CreateUserResponse {
	return CreateUserResponse{
		ID:        user.ID,
		Username:  user.Username,
		SessionID: session.ID,
		ExpiresAt: session.ExpiresAt,
	}
}
