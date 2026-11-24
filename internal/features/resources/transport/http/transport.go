package resources_transport_http

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	"github.com/vladpann/go-cloud-file-storage/config"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/resources/transport/http")

// ResourcesHTTPHandler handles HTTP requests for resource operations
type ResourcesHTTPHandler struct {
	resourcesService ResourcesService
	config           *config.Config
	l                logger.Interface
	validator        *validator.Validate
}

type ResourcesService interface {
	CreateEmptyFolder(
		ctx context.Context,
		userID uuid.UUID,
		rawPath string,
	) (domain.Resource, error)
}

func NewResourcesHTTPHandler(
	resourcesService ResourcesService,
	cfg *config.Config,
	l logger.Interface,
	v *validator.Validate,
) *ResourcesHTTPHandler {
	return &ResourcesHTTPHandler{
		resourcesService: resourcesService,
		config:           cfg,
		l:                l,
		validator:        v,
	}
}
