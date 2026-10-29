package app

import (
	"net/http"

	"example.com/trest/services/project-service/internal/application"
	"example.com/trest/services/project-service/internal/plan"
	"example.com/trest/services/project-service/internal/project"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	registerProjectRoutes(mux, pool)
	registerApplicationRoutes(mux, pool)
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
