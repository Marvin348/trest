package application

import (
	"context"
	"errors"
	"fmt"

	"example.com/trest/services/project-service/internal/apperror"
	"example.com/trest/services/project-service/internal/plan"
	"example.com/trest/services/project-service/internal/project"
)

type Service struct {
	repo        *Repository
	projectRepo *project.Repository
	planRepo    *plan.Repository
}

func NewService(repo *Repository, projectRepo *project.Repository,
	planRepo *plan.Repository) *Service {
	return &Service{
		repo:        repo,
		projectRepo: projectRepo,
		planRepo:    planRepo,
	}
}

func (s *Service) Create(ctx context.Context, params CreateApplicationParams) (Application, error) {
	planData, err := s.planRepo.GetByID(ctx, params.PlanID)
	if errors.Is(err, plan.ErrNotFound) {
		return Application{}, &apperror.AppError{
			Code:    apperror.NotFound,
			Message: "Plan not found",
		}
	}
	if err != nil {
		return Application{}, fmt.Errorf("get application plan: %w", err)
	}

	data := CreateApplicationData{
		ProjectID:       params.ProjectID,
		PlanID:          planData.ID,
		Name:            params.Name,
		Image:           params.Image,
		ContainerPort:   params.ContainerPort,
		Replicas:        planData.Replicas,
		CPURequestM:     planData.CPURequestM,
		CPULimitM:       planData.CPULimitM,
		MemoryRequestMi: planData.MemoryRequestMi,
		MemoryLimitMi:   planData.MemoryLimitMi,
	}

	application, err := s.repo.Create(ctx, data)

	if err != nil {
		switch {
		case errors.Is(err, ErrDuplicateApplication):
			return Application{}, &apperror.AppError{
				Code:    apperror.Conflict,
				Message: "Application already exists",
			}

		case errors.Is(err, ErrProjectNotFound):
			return Application{}, &apperror.AppError{
				Code:    apperror.NotFound,
				Message: "Project not found",
			}

		case errors.Is(err, ErrPlanNotFound):
			return Application{}, &apperror.AppError{
				Code:    apperror.NotFound,
				Message: "Plan not found",
			}

		default:
			return Application{}, fmt.Errorf("create application: %w", err)
		}
	}

	return application, nil
}
