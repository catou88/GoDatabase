package controllers

import (
	"context"
	"encoding/json"
	"godatabase/internal/server/models"
	"godatabase/internal/server/response"
	"io"
	"net/http"
)

type ExperimentRunner interface {
	Run(context.Context, models.ExperimentRequest) (models.ExperimentResult, error)
}
type ExperimentController struct {
	Runner              ExperimentRunner
	MaxBodyBytes        int64
	MaxOperations       int
	MaxDataset          int
	MaxTraceEvents      int
	SupportedStructures map[string]struct{}
}

func (c ExperimentController) Run(w http.ResponseWriter, r *http.Request) {
	if c.Runner == nil {
		response.Error(w, http.StatusServiceUnavailable, "experiment_unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, c.MaxBodyBytes)
	var request models.ExperimentRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || !validRequest(request, c.MaxOperations, c.MaxDataset, c.SupportedStructures) {
		response.Error(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := c.Runner.Run(context.Background(), request)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, "experiment_failed")
		return
	}
	if c.MaxTraceEvents > 0 && len(result.Trace) > c.MaxTraceEvents {
		result.Trace = result.Trace[:c.MaxTraceEvents]
	}
	response.JSON(w, http.StatusOK, result)
}
func validRequest(request models.ExperimentRequest, maxOperations, maxDataset int, supported map[string]struct{}) bool {
	if request.DatasetSize < 0 || request.DatasetSize > maxDataset || len(request.Operations) > maxOperations || request.Seed < 0 {
		return false
	}
	if _, ok := supported[string(request.Structure)]; !ok {
		return false
	}
	for _, operation := range request.Operations {
		switch operation.Name {
		case "set", "get", "delete":
			if operation.Key == "" {
				return false
			}
		case "range":
			if operation.Start > operation.End {
				return false
			}
		default:
			return false
		}
	}
	return true
}
