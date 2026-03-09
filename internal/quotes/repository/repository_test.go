package repository

import (
	"math"
	"testing"
)

func TestToPgNumericValueReturnsValidNumericForFiniteQuantity(t *testing.T) {
	numeric := toPgNumericValue(1.5)
	if !numeric.Valid {
		t.Fatal("expected numeric to be valid")
	}
	value, err := numericFloat64(numeric)
	if err != nil {
		t.Fatalf("numericFloat64 returned error: %v", err)
	}
	if value != 1.5 {
		t.Fatalf("expected 1.5, got %v", value)
	}
}

func TestToPgNumericValueFallsBackForInvalidQuantity(t *testing.T) {
	numeric := toPgNumericValue(math.NaN())
	if !numeric.Valid {
		t.Fatal("expected fallback numeric to be valid")
	}
	value, err := numericFloat64(numeric)
	if err != nil {
		t.Fatalf("numericFloat64 returned error: %v", err)
	}
	if value != 1 {
		t.Fatalf("expected fallback quantity 1, got %v", value)
	}
}
