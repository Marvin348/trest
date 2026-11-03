package deployment

import (
	"context"
	"errors"
	"fmt"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, params CreateDeploymentParams) (Deployment, error) {

	if err := validateCreateDeploymentParams(params); err != nil {
		return Deployment{}, err
	}

	created, err := s.repo.Create(ctx, params)

	if errors.Is(err, ErrAlreadyExists) {
		existing, err := s.repo.GetByRequestID(ctx, params.RequestID)
		if err != nil {
			return Deployment{}, fmt.Errorf("get existing deployment: %w", err)
		}

		if !sameDeploymentRequest(existing, params) {
			return Deployment{}, ErrIdempotencyConflict
		}

		return existing, nil
	}

	if err != nil {
		return Deployment{}, fmt.Errorf("create deployment: %w", err)
	}

	return created, nil
}
