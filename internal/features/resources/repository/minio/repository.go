package resources_repository_minio

import (
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	pkgminio "github.com/vladpann/go-cloud-file-storage/pkg/minio"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/resources/repository/minio")

type ResourcesRepository struct {
	mc *pkgminio.Client
}

func New(
	mc *pkgminio.Client,
) *ResourcesRepository {
	return &ResourcesRepository{
		mc: mc,
	}
}

func userRootFolderKey(userID uuid.UUID) string {
	return fmt.Sprintf("user-%s-files/", userID.String())
}
