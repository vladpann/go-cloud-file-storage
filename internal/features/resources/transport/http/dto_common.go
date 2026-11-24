package resources_transport_http

import "github.com/vladpann/go-cloud-file-storage/internal/core/domain"

// CreateEmptyFolderResponse represents the create empty folder response
type CreateEmptyFolderResponse struct {
	Path string              `json:"path"`
	Name string              `json:"name"`
	Type domain.ResourceType `json:"type"`
}

// createEmptyFolderResponseFromDomain converts a domain resource into the create empty folder response DTO
func createEmptyFolderResponseFromDomain(resource domain.Resource) CreateEmptyFolderResponse {
	return CreateEmptyFolderResponse{
		Path: resource.Path,
		Name: resource.Name,
		Type: resource.Type,
	}
}
