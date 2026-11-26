package resources_transport_http

import "github.com/vladpann/go-cloud-file-storage/internal/core/domain"

// CreateEmptyFolderResponse represents the creation empty folder response
type CreateEmptyFolderResponse struct {
	Path string              `json:"path"`
	Name string              `json:"name"`
	Type domain.ResourceType `json:"type"`
}

// GetFolderContent represents the get folder content response
type GetFolderContent struct {
	Path string              `json:"path"`
	Name string              `json:"name"`
	Size *int64              `json:"size,omitempty"`
	Type domain.ResourceType `json:"type"`
}

// createEmptyFolderResponseFromDomain converts a domain resource into the creation empty folder response DTO
func createEmptyFolderResponseFromDomain(resource domain.Resource) CreateEmptyFolderResponse {
	return CreateEmptyFolderResponse{
		Path: resource.Path,
		Name: resource.Name,
		Type: resource.Type,
	}
}

// getFolderContentResponseFromDomain converts a domain resource into the get folder content response DTO
func getFolderContentResponseFromDomain(resources []domain.Resource) []GetFolderContent {
	info := make([]GetFolderContent, 0, len(resources))
	for _, resource := range resources {
		info = append(info, GetFolderContent{
			Path: resource.Path,
			Name: resource.Name,
			Size: resource.Size,
			Type: resource.Type,
		})
	}

	return info
}
