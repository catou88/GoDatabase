package controllers

import (
	"context"
	"godatabase/internal/server/models"
)

type ExperimentRunner interface {
	Run(context.Context, models.ExperimentRequest) (models.ExperimentResult, error)
}
