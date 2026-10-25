package app

import (
	"net/http"

	"example.com/trest/services/project-service/internal/project"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	registerProjectRoutes(mux, pool)
}

func registerProjectRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	repo := project.NewRepository(pool)
	service := project.NewService(repo)
	handler := project.NewHandler(service)

	project.RegisterProjectRoutes(mux, handler)
}
