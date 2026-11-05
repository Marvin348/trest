package app

import (
	"net/http"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/project-service/internal/application"
	"example.com/trest/services/project-service/internal/deployment"
	"example.com/trest/services/project-service/internal/plan"
	"example.com/trest/services/project-service/internal/project"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB               *pgxpool.Pool
	DeploymentClient deploymentv1.DeploymentServiceClient
}

func RegisterRoutes(mux *http.ServeMux, deps Dependencies) {
	registerProjectRoutes(mux, deps.DB)
	registerApplicationRoutes(mux, deps.DB)
	registerDeploymentRoutes(mux, deps)
}

func registerProjectRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	repo := project.NewRepository(pool)
	service := project.NewService(repo)
	handler := project.NewHandler(service)

	project.RegisterProjectRoutes(mux, handler)
}

func registerApplicationRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	repo := application.NewRepository(pool)
	projectRepo := project.NewRepository(pool)
	planRepo := plan.NewRepository(pool)

	service := application.NewService(repo, projectRepo, planRepo)
	handler := application.NewHandler(service)

	application.RegisterRoutes(mux, handler)
}

func registerDeploymentRoutes(mux *http.ServeMux, deps Dependencies) {
	applicationRepo := application.NewRepository(deps.DB)

	service := deployment.NewService(
		applicationRepo, deps.DeploymentClient)

	handler := deployment.NewHandler(service)

	deployment.RegisterRoutes(mux, handler)
}
