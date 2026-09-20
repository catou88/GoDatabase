package controllers

import (
	"context"
	"godatabase/internal/experiment"
)

type ExperimentRunner interface {
	Run(context.Context, experiment.ExperimentRequest) (experiment.ExperimentResult, error)
}
