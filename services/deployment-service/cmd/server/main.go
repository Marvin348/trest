package main

import (
	"example.com/trest/services/deployment-service/internal/app"
	"log/slog"
	"os"
)

func main() {
	if err := app.Run(); err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}
