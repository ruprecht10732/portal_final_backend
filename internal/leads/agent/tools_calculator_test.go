package agent

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

func TestHandleCalculatorExpression(t *testing.T) {
	output, err := handleCalculator(nil, CalculatorInput{
		Expression: "((15.99 * 3) + (12.50 * 2)) * 1.21",
	})
	if err != nil {
		t.Fatalf("expected expression evaluation without error, got %v", err)
	}

	const want = 88.2937
	if math.Abs(output.Result-want) > 1e-9 {
		t.Fatalf("expected expression result %v, got %v", want, output.Result)
	}
	if !strings.Contains(output.Expression, "((15.99 * 3) + (12.50 * 2)) * 1.21 =") {
		t.Fatalf("expected output expression to include original expression, got %q", output.Expression)
	}
}

func TestHandleCalculatorExpressionFunctions(t *testing.T) {
	output, err := handleCalculator(nil, CalculatorInput{
		Expression: "round(percentage(99.99, 21) + ceil_divide(4, 2.5), 2)",
	})
	if err != nil {
		t.Fatalf("expected helper-function expression evaluation without error, got %v", err)
	}

	const want = 23.00
	if math.Abs(output.Result-want) > 1e-9 {
		t.Fatalf("expected helper-function result %v, got %v", want, output.Result)
	}
}

func TestHandleCalculatorLegacyOperationFallback(t *testing.T) {
	output, err := handleCalculator(nil, CalculatorInput{
		Operation: "multiply",
		A:         15.99,
		B:         3,
	})
	if err != nil {
		t.Fatalf("expected legacy calculator operation without error, got %v", err)
	}

	const want = 47.97
	if math.Abs(output.Result-want) > 1e-9 {
		t.Fatalf("expected legacy result %v, got %v", want, output.Result)
	}
}

func TestHandleCalculatorRejectsUnsupportedExpressionSyntax(t *testing.T) {
	_, err := handleCalculator(nil, CalculatorInput{
		Expression: "pow(2, 3)",
	})
	if err == nil {
		t.Fatalf("expected unsupported function to return an error")
	}
	if !strings.Contains(err.Error(), "unsupported function") {
		t.Fatalf("expected unsupported function error, got %v", err)
	}

	_, err = handleCalculator(nil, CalculatorInput{
		Expression: "1 << 2",
	})
	if err == nil {
		t.Fatalf("expected unsupported operator to return an error")
	}
	if !strings.Contains(err.Error(), "unsupported operator") {
		t.Fatalf("expected unsupported operator error, got %v", err)
	}
}

func TestCreateCalculatorToolProcessRequestIncludesExpressionSchema(t *testing.T) {
	calculator, err := createCalculatorTool()
	if err != nil {
		t.Fatalf("expected calculator tool creation without error, got %v", err)
	}

	requestProcessor, ok := calculator.(interface {
		ProcessRequest(tool.Context, *model.LLMRequest) error
	})
	if !ok {
		t.Fatalf("expected calculator tool to implement ProcessRequest")
	}

	var req model.LLMRequest
	if err := requestProcessor.ProcessRequest(nil, &req); err != nil {
		t.Fatalf("expected calculator tool to pack request without error, got %v", err)
	}
	if req.Config == nil || len(req.Config.Tools) == 0 || req.Config.Tools[0] == nil {
		t.Fatalf("expected calculator tool to add function declarations to request config")
	}
	if len(req.Config.Tools[0].FunctionDeclarations) == 0 || req.Config.Tools[0].FunctionDeclarations[0] == nil {
		t.Fatalf("expected calculator tool to provide at least one function declaration")
	}

	declaration := req.Config.Tools[0].FunctionDeclarations[0]
	if declaration.Name != "Calculator" {
		t.Fatalf("expected calculator declaration name, got %q", declaration.Name)
	}

	schemaBytes, err := json.Marshal(declaration.ParametersJsonSchema)
	if err != nil {
		t.Fatalf("expected calculator parameter schema to marshal, got %v", err)
	}
	schemaText := string(schemaBytes)
	for _, token := range []string{"\"expression\"", "\"operation\"", "\"a\"", "\"b\""} {
		if !strings.Contains(schemaText, token) {
			t.Fatalf("expected calculator parameter schema to contain %s, got %s", token, schemaText)
		}
	}
}
