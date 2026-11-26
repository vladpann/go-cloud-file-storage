package resources_transport_http

import "github.com/vladpann/go-cloud-file-storage/internal/core/domain"

// CreateEmptyFolderResponse represents the creation empty folder response
type CreateEmptyFolderResponse struct {
	Path string              `json:"path"`
	Name string              `json:"name"`
	Type domain.ResourceType `json:"type"`
}

// GetFolderContentResponse represents the get folder content response
type GetFolderContentResponse struct {
	Path string              `json:"path"`
	Name string              `json:"name"`
	Size *int64              `json:"size,omitempty"`
	Type domain.ResourceType `json:"type"`
}

// UploadResourceResponse represents the upload resource
type UploadResourceResponse struct {
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
func getFolderContentResponseFromDomain(resources []domain.Resource) []GetFolderContentResponse {
	info := make([]GetFolderContentResponse, 0, len(resources))
	for _, resource := range resources {
		info = append(info, GetFolderContentResponse{
			Path: resource.Path,
			Name: resource.Name,
			Size: resource.Size,
			Type: resource.Type,
		})
	}

	return info
}

// uploadResourceResponse converts a domain resource into the upload resource response DTO
func uploadResourceResponseFromDomain(resources []domain.Resource) []UploadResourceResponse {
	info := make([]UploadResourceResponse, 0, len(resources))
	for _, resource := range resources {
		info = append(info, UploadResourceResponse{
			Path: resource.Path,
			Name: resource.Name,
			Size: resource.Size,
			Type: resource.Type,
		})
	}

	return info
}
