package resources_service

import (
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	"github.com/vladpann/go-cloud-file-storage/config"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/resources/service")

type ResourcesService struct {
	cfg                 *config.Config
	resourcesRepository ResourcesRepository
}

type ResourcesRepository interface {
	CreateParentFolder(
		ctx context.Context,
		userID uuid.UUID,
	) error

	CreateFolder(
		ctx context.Context,
		userID uuid.UUID,
		dir domain.Resource,
	) error
}

func New(
	cfg *config.Config,
	resourcesRepository ResourcesRepository,
) *ResourcesService {
	return &ResourcesService{
		cfg:                 cfg,
		resourcesRepository: resourcesRepository,
	}
}
