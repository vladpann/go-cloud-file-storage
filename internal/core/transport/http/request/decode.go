package core_http_request

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"

	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
)

const (
	// MaxRequestBodySize defines maximum allowed request body size (10MB)
	MaxRequestBodySize int64 = 10 << 20
)

// Validatable defines interface for structs with custom validation logic
type Validatable interface {
	Validate() error
}

// ValidationError represents a validation error with field details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// DecodeAndValidateRequest decodes JSON request body and validates it
// It limits request body size, handles JSON decode errors, and performs validation
func DecodeAndValidateRequest(r *http.Request, dest any, v *validator.Validate) error {
	// Limit request body size to prevent DoS attacks
	r.Body = http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)

	// Decode JSON with better error handling
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Reject unknown fields in JSON

	if err := decoder.Decode(dest); err != nil {
		return formatDecodeError(err)
	}

	// Validate the decoded struct
	return validateStruct(dest, v)
}

// formatDecodeError converts JSON decode errors to user-friendly messages
func formatDecodeError(err error) error {
	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.As(err, &syntaxError):
		return fmt.Errorf(
			"malformed JSON at position %d: %w",
			syntaxError.Offset,
			core_errors.ErrInvalidArgument,
		)

	case errors.As(err, &unmarshalTypeError):
		return fmt.Errorf(
			"invalid type for field '%s': expected %s: %w",
			unmarshalTypeError.Field,
			unmarshalTypeError.Type.String(),
			core_errors.ErrInvalidArgument,
		)

	case errors.As(err, &maxBytesError):
		return fmt.Errorf(
			"request body too large (max %d bytes): %w",
			MaxRequestBodySize,
			core_errors.ErrInvalidArgument,
		)

	case errors.Is(err, io.EOF):
		return fmt.Errorf(
			"request body is empty: %w",
			core_errors.ErrInvalidArgument,
		)

	case strings.Contains(err.Error(), "unknown field"):
		return fmt.Errorf(
			"request contains unknown fields: %w",
			core_errors.ErrInvalidArgument,
		)

	default:
		return fmt.Errorf(
			"failed to decode request: %v: %w",
			err,
			core_errors.ErrInvalidArgument,
		)
	}
}

// validateStruct validates struct using validator or custom Validate method
func validateStruct(dest any, v *validator.Validate) error {
	// Check if struct implements custom validation
	if val, ok := dest.(Validatable); ok {
		if err := val.Validate(); err != nil {
			return fmt.Errorf(
				"validation failed: %v: %w",
				err,
				core_errors.ErrInvalidArgument,
			)
		}
		return nil
	}

	// Use validator tags
	if err := v.Struct(dest); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// formatValidationError converts validator errors to user-friendly format
func formatValidationError(err error) error {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		errMessages := make([]string, 0, len(validationErrs))
		for _, fieldErr := range validationErrs {
			errMessages = append(errMessages, formatFieldError(fieldErr))
		}

		return fmt.Errorf(
			"validation failed: %s: %w",
			strings.Join(errMessages, "; "),
			core_errors.ErrInvalidArgument,
		)
	}

	return fmt.Errorf(
		"validation failed: %v: %w",
		err,
		core_errors.ErrInvalidArgument,
	)
}

// formatFieldError creates human-readable error message for a field validation error
func formatFieldError(err validator.FieldError) string {
	field := strings.ToLower(err.Field())

	switch err.Tag() {
	case "required":
		return fmt.Sprintf("field '%s' is required", field)
	case "email":
		return fmt.Sprintf("field '%s' must be a valid email", field)
	case "min":
		return fmt.Sprintf("field '%s' must be at least %s characters", field, err.Param())
	case "max":
		return fmt.Sprintf("field '%s' must be at most %s characters", field, err.Param())
	case "len":
		return fmt.Sprintf("field '%s' must be exactly %s characters", field, err.Param())
	case "gt":
		return fmt.Sprintf("field '%s' must be greater than %s", field, err.Param())
	case "gte":
		return fmt.Sprintf("field '%s' must be greater than or equal to %s", field, err.Param())
	case "lt":
		return fmt.Sprintf("field '%s' must be less than %s", field, err.Param())
	case "lte":
		return fmt.Sprintf("field '%s' must be less than or equal to %s", field, err.Param())
	case "oneof":
		return fmt.Sprintf("field '%s' must be one of: %s", field, err.Param())
	case "uuid":
		return fmt.Sprintf("field '%s' must be a valid UUID", field)
	case "url":
		return fmt.Sprintf("field '%s' must be a valid URL", field)
	default:
		return fmt.Sprintf("field '%s' failed validation '%s'", field, err.Tag())
	}
}
