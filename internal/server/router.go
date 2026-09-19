package server

import (
	"godatabase/internal/metrics"
	"godatabase/internal/server/controllers"
	"godatabase/internal/server/middleware"
	"net/http"
)

type Config struct {
	Runner        controllers.ExperimentRunner
	Complexity    metrics.Catalog
	AllowedOrigin string
	MaxBodyBytes  int64
	MaxOperations int
	MaxDataset    int
}

func New(config Config) http.Handler {
	if config.Complexity == nil {
		config.Complexity = metrics.DefaultCatalog()
	}
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = 1 << 20
	}
	if config.MaxOperations <= 0 {
		config.MaxOperations = 1000
	}
	if config.MaxDataset <= 0 {
		config.MaxDataset = 100000
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/structures", controllers.StructureController{Complexity: config.Complexity}.List)
	mux.HandleFunc("POST /api/v1/experiments", (controllers.ExperimentController{Runner: config.Runner, MaxBodyBytes: config.MaxBodyBytes, MaxOperations: config.MaxOperations, MaxDataset: config.MaxDataset}).Run)
	return middleware.CORS(config.AllowedOrigin)(mux)
}
