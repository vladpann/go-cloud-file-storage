package resources_transport_http

import (
	"github.com/go-playground/validator/v10"
	"github.com/vladpann/go-cloud-file-storage/config"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

// ResourcesHTTPHandler handles HTTP requests for resource operations
type ResourcesHTTPHandler struct {
	resourcesService ResourcesService
	config           *config.Config
	l                logger.Interface
	validator        *validator.Validate
}

type ResourcesService struct{}
