package repository

import (
	"strings"
	"testing"
)

func TestAIAnalysisSelectColumnsIncludesConfidenceFields(t *testing.T) {
	for _, column := range requiredAIAnalysisColumns() {
		if !strings.Contains(aiAnalysisSelectColumns, column) {
			t.Fatalf("expected aiAnalysisSelectColumns to contain %q", column)
		}
	}
}