package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"godatabase/internal/metrics"
	"godatabase/internal/server/models"
)

type fakeRunner struct {
	request models.ExperimentRequest
	err     error
}

func (f *fakeRunner) Run(_ context.Context, request models.ExperimentRequest) (models.ExperimentResult, error) {
	f.request = request
	if f.err != nil {
		return models.ExperimentResult{}, f.err
	}
	return models.ExperimentResult{Structure: request.Structure, Results: []models.OperationResult{{Found: true, Value: "ok"}}}, nil
}

func TestStructuresEndpointReturnsComplexityWithoutInternals(t *testing.T) {
	recorder := httptest.NewRecorder()
	New(Config{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/structures", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response []struct {
		Name       metrics.Structure   `json:"name"`
		Operations []metrics.Operation `json:"operations"`
		Complexity []metrics.Metadata  `json:"complexity"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 4 || len(response[0].Operations) != 4 || len(response[0].Complexity) != 4 {
		t.Fatalf("unexpected structure response: %#v", response)
	}
	if stringsContains(recorder.Body.String(), "*btree") {
		t.Fatal("response exposed an internal implementation type")
	}
}

func TestExperimentEndpointValidatesAndRunsRequest(t *testing.T) {
	runner := &fakeRunner{}
	handler := New(Config{Runner: runner, AllowedOrigin: "http://localhost:3000"})
	body := `{"structure":"btree-memory","dataset_size":10,"seed":7,"operations":[{"name":"get","key":"alpha"}]}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/experiments", strings.NewReader(body))
	request.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("status/headers = %d/%q", recorder.Code, recorder.Header().Get("Access-Control-Allow-Origin"))
	}
	if runner.request.Seed != 7 || runner.request.Structure != metrics.BTreeMemory {
		t.Fatalf("runner request = %#v", runner.request)
	}
}

func TestExperimentEndpointHidesRunnerErrors(t *testing.T) {
	handler := New(Config{Runner: &fakeRunner{err: errors.New("disk page 7 checksum mismatch")}})
	body := `{"structure":"map","dataset_size":1,"seed":1,"operations":[]}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/experiments", strings.NewReader(body)))
	if recorder.Code != http.StatusUnprocessableEntity || stringsContains(recorder.Body.String(), "checksum") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestExperimentEndpointRejectsInvalidRequests(t *testing.T) {
	runner := &fakeRunner{}
	handler := New(Config{Runner: runner, MaxOperations: 1})
	body := `{"structure":"map","dataset_size":1,"seed":1,"operations":[{"name":"drop","key":"a"}]}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/experiments", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !reflect.DeepEqual(runner.request, models.ExperimentRequest{}) {
		t.Fatal("runner was called for invalid request")
	}
}

func stringsContains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
