package service

import (
	"context"

	geninfo "digital-contracting-service/gen/info"
	"digital-contracting-service/internal/buildinfo"
)

type infoSrvc struct{}

// NewInfo returns the Info service implementation.
func NewInfo() geninfo.Service {
	return &infoSrvc{}
}

func (s *infoSrvc) Get(_ context.Context) (*geninfo.GetResult, error) {
	res := &geninfo.GetResult{GitSha: buildinfo.GitSHA}
	if buildinfo.BuiltAt != "" {
		res.BuiltAt = &buildinfo.BuiltAt
	}
	return res, nil
}
