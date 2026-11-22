package domain

import (
	"errors"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid or missing path")
	ErrInvalidSize = errors.New("file size cannot be negative")
)

type ResourceType string

const (
	ResourceTypeFile      ResourceType = "FILE"
	ResourceTypeDirectory ResourceType = "DIRECTORY"
)

type Resource struct {
	Path string
	Name string
	Size *int64
	Type ResourceType
}

func NewFile(parentPath, name string, size int64) (Resource, error) {
	if err := validateParentPath(parentPath); err != nil {
		return Resource{}, err
	}
	if strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
		return Resource{}, ErrInvalidPath
	}
	if size < 0 {
		return Resource{}, ErrInvalidSize
	}

	return Resource{
		Path: parentPath,
		Name: name,
		Size: &size,
		Type: ResourceTypeFile,
	}, nil
}

func NewDirectory(parentPath, name string) (Resource, error) {
	if err := validateParentPath(parentPath); err != nil {
		return Resource{}, err
	}
	if strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
		return Resource{}, ErrInvalidPath
	}

	return Resource{
		Path: parentPath,
		Name: name,
		Size: nil,
		Type: ResourceTypeDirectory,
	}, nil
}

// FullPath returns the full path of a resource (for folders ending in “/”)
func (r Resource) FullPath() string {
	if r.Type == ResourceTypeDirectory {
		return r.Path + r.Name + "/"
	}
	return r.Path + r.Name
}

func (t ResourceType) IsValid() bool {
	return t == ResourceTypeFile || t == ResourceTypeDirectory
}

func (r Resource) IsDir() bool {
	return r.Type == ResourceTypeDirectory
}

func validateParentPath(p string) error {
	if p == "" {
		return nil
	}
	if strings.HasPrefix(p, "/") || !strings.HasSuffix(p, "/") || strings.Contains(p, "//") {
		return ErrInvalidPath
	}
	return nil
}

// ParseResourcePath parses a full path (for example, “folder1/folder2/file.txt” or “folder1/folder2/”)
// into parentPath, name, and ResourceType
func ParseResourcePath(rawPath string) (parentPath, name string, resType ResourceType, err error) {
	if rawPath == "" || rawPath == "/" || strings.HasPrefix(rawPath, "/") || strings.Contains(rawPath, "//") {
		return "", "", "", ErrInvalidPath
	}

	// Check for path traversal
	for seg := range strings.SplitSeq(strings.TrimSuffix(rawPath, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", "", "", ErrInvalidPath
		}
	}

	if strings.HasSuffix(rawPath, "/") {
		resType = ResourceTypeDirectory
		trimmed := strings.TrimSuffix(rawPath, "/")
		idx := strings.LastIndex(trimmed, "/")
		if idx == -1 {
			return "", trimmed, resType, nil
		}
		return trimmed[:idx+1], trimmed[idx+1:], resType, nil
	}

	resType = ResourceTypeFile
	idx := strings.LastIndex(rawPath, "/")
	if idx == -1 {
		return "", rawPath, resType, nil
	}
	return rawPath[:idx+1], rawPath[idx+1:], resType, nil
}
