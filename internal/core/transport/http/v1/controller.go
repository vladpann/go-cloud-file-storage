package core_http_v1

import (
	"github.com/go-playground/validator/v10"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

type V1 struct {
	l logger.Interface
	v *validator.Validate
}
