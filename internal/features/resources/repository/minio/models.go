package resources_repository_minio

import "github.com/vladpann/go-cloud-file-storage/internal/core/domain"

type ResourceModel struct {
	Path string
	Name string
	Size *int64
	Type domain.ResourceType
}

func resourceModelFromDomain(resource domain.Resource) ResourceModel {
	return ResourceModel{
		Path: resource.Path,
		Name: resource.Name,
		Size: resource.Size,
		Type: resource.Type,
	}
}

func resourceDomainsFromModel(resources []ResourceModel) []domain.Resource {
	res := make([]domain.Resource, 0, len(resources))

	for _, r := range resources {
		res = append(res, domain.Resource{
			Path: r.Path,
			Name: r.Name,
			Size: r.Size,
			Type: r.Type,
		})
	}

	return res
}
