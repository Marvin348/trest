package project

import (
	"context"
	"fmt"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Create(ctx context.Context, name string) (Project, error) {
	project, err := s.repo.Create(ctx, name)

	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}

	return project, nil
}
