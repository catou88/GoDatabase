package controllers

import (
	"godatabase/internal/metrics"
	"godatabase/internal/server/response"
	"net/http"
)

type StructureController struct{ Complexity metrics.Catalog }

func (c StructureController) List(w http.ResponseWriter, _ *http.Request) {
	operations := []metrics.Operation{metrics.Set, metrics.Get, metrics.Delete, metrics.Range}
	structures := []metrics.Structure{metrics.Map, metrics.SortedSlice, metrics.BTreeMemory, metrics.BTreeDurable}
	type item struct {
		Name       metrics.Structure   `json:"name"`
		Operations []metrics.Operation `json:"operations"`
		Complexity []metrics.Metadata  `json:"complexity"`
	}
	result := make([]item, 0, len(structures))
	for _, structure := range structures {
		metadata := make([]metrics.Metadata, 0, len(operations))
		for _, operation := range operations {
			if value, ok := c.Complexity.Lookup(structure, operation); ok {
				metadata = append(metadata, value)
			}
		}
		result = append(result, item{structure, operations, metadata})
	}
	response.JSON(w, http.StatusOK, result)
}
