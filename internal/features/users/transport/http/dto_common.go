package users_transport_http

import (
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

// SignUpRequest represents the sign-up request
type SignUpRequest struct {
	Username       string `json:"username" validate:"required"`
	Password       string `json:"password" validate:"required,min=5,max=72"`
	RepeatPassword string `json:"repeat_password" validate:"required"`
}

// SignUpResponse represents the sign-up response
type SignUpResponse struct {
	Username string `json:"username"`
}

// SignInRequest represents the sign-in request
type SignInRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// SignInResponse represents the sign-in response
type SignInResponse struct {
	Username string `json:"username"`
}

// MeResponse represents the me response
type MeResponse struct {
	Username string `json:"username"`
}

// signUpResponseFromDomain converts a domain user into the sign-up response DTO
func signUpResponseFromDomain(user domain.User) SignUpResponse {
	return SignUpResponse{
		Username: user.Username,
	}
}

// signInResponseFromDomain converts a domain user into the sign-in response DTO
func signInResponseFromDomain(user domain.User) SignInResponse {
	return SignInResponse{
		Username: user.Username,
	}
}
