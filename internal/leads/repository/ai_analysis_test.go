package repository

import (
	"strings"
	"testing"
)

func TestAIAnalysisSelectColumnsIncludesConfidenceFields(t *testing.T) {
	for _, column := range []string{
		"composite_confidence",
		"confidence_breakdown",
		"risk_flags",
		"resolved_information",
		"extracted_facts",
	} {
		if !strings.Contains(aiAnalysisSelectColumns, column) {
			t.Fatalf("expected aiAnalysisSelectColumns to contain %q", column)
		}
	}
}
