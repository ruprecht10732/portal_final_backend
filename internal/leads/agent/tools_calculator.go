package agent

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"math"
	"portal_final_backend/internal/leads/repository"
	"strconv"
	"strings"

	"google.golang.org/adk/tool"
	apptools "portal_final_backend/internal/tools"
)

func createSaveEstimationTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSaveEstimationTool(func(ctx tool.Context, input SaveEstimationInput) (SaveEstimationOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return SaveEstimationOutput{}, err
		}
		tenantID, err := getTenantID(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: missingLeadContextMessage}, err
		}

		actorType, actorName := deps.GetActor()
		summary := strings.TrimSpace(input.Summary)
		var summaryPtr *string
		if summary != "" {
			summaryPtr = &summary
		}

		_, err = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      repository.EventTypeAnalysis,
			Title:          repository.EventTitleEstimationSaved,
			Summary:        summaryPtr,
			Metadata: repository.EstimationMetadata{
				Scope:      input.Scope,
				PriceRange: input.PriceRange,
				Notes:      input.Notes,
			}.ToMap(),
		})
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: "Failed to save estimation"}, err
		}

		deps.SetLastEstimationMetadata(repository.EstimationMetadata{
			Scope:      input.Scope,
			PriceRange: input.PriceRange,
			Notes:      input.Notes,
		}.ToMap())
		deps.MarkSaveEstimationCalled()

		return SaveEstimationOutput{Success: true, Message: "Estimation saved"}, nil
	})
}

func createCommitScopeArtifactTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewCommitScopeArtifactTool(func(ctx tool.Context, input CommitScopeArtifactInput) (CommitScopeArtifactOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return CommitScopeArtifactOutput{}, err
		}
		artifact := input.Artifact
		if len(artifact.MissingDimensions) > 0 {
			artifact.IsComplete = false
		}
		if artifact.WorkItems == nil {
			artifact.WorkItems = make([]ScopeWorkItem, 0)
		}
		deps.SetScopeArtifact(artifact)
		return CommitScopeArtifactOutput{Success: true, Message: "Scope artifact opgeslagen"}, nil
	})
}

func createAskCustomerClarificationTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewAskCustomerClarificationTool(func(ctx tool.Context, input AskCustomerClarificationInput) (AskCustomerClarificationOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return AskCustomerClarificationOutput{}, err
		}
		tenantID, err := getTenantID(deps)
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: missingLeadContextMessage}, err
		}

		message := strings.TrimSpace(input.Message)
		if message == "" {
			return AskCustomerClarificationOutput{Success: false, Message: "Bericht is verplicht"}, fmt.Errorf("empty clarification message")
		}

		if len([]rune(message)) > 1200 {
			message = truncateRunes(message, 1200)
		}

		actorType, actorName := deps.GetActor()
		_, err = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      repository.EventTypeNote,
			Title:          repository.EventTitleNoteAdded,
			Summary:        &message,
			Metadata: map[string]any{
				"noteType":          "ai_clarification_request",
				"missingDimensions": input.MissingDimensions,
			},
		})
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: "Kon verduidelijkingsvraag niet opslaan"}, err
		}

		deps.MarkClarificationAsked()
		log.Printf("estimator AskCustomerClarification: run=%s lead=%s service=%s missing=%d", deps.GetRunID(), leadID, serviceID, len(input.MissingDimensions))
		return AskCustomerClarificationOutput{Success: true, Message: "Verduidelijkingsvraag opgeslagen"}, nil
	})
}

// handleCalculator evaluates a single arithmetic operation deterministically.
// The LLM MUST call this for ANY math instead of doing it in its head.
// The LLM MUST call this for ANY math instead of doing it in its head.
func handleCalculator(_ tool.Context, input CalculatorInput) (CalculatorOutput, error) {
	expression := strings.TrimSpace(input.Expression)
	if expression != "" {
		result, err := evaluateCalculatorExpression(expression)
		if err != nil {
			return CalculatorOutput{}, err
		}
		return CalculatorOutput{
			Result:     result,
			Expression: fmt.Sprintf("%s = %g", expression, result),
		}, nil
	}

	return evaluateLegacyCalculatorOperation(input)
}

func evaluateLegacyCalculatorOperation(input CalculatorInput) (CalculatorOutput, error) {
	var result float64
	var expr string

	switch strings.ToLower(strings.TrimSpace(input.Operation)) {
	case "add":
		result = input.A + input.B
		expr = fmt.Sprintf("%g + %g = %g", input.A, input.B, result)
	case "subtract":
		result = input.A - input.B
		expr = fmt.Sprintf("%g - %g = %g", input.A, input.B, result)
	case "multiply":
		result = input.A * input.B
		expr = fmt.Sprintf("%g × %g = %g", input.A, input.B, result)
	case "divide":
		if input.B == 0 {
			return CalculatorOutput{}, errors.New(divisionByZeroMessage)
		}
		result = input.A / input.B
		expr = fmt.Sprintf("%g ÷ %g = %g", input.A, input.B, result)
	case "ceil_divide":
		if input.B == 0 {
			return CalculatorOutput{}, errors.New(divisionByZeroMessage)
		}
		result = math.Ceil(input.A / input.B)
		expr = fmt.Sprintf("⌈%g ÷ %g⌉ = %g", input.A, input.B, result)
	case "ceil":
		result = math.Ceil(input.A)
		expr = fmt.Sprintf("⌈%g⌉ = %g", input.A, result)
	case "floor":
		result = math.Floor(input.A)
		expr = fmt.Sprintf("⌊%g⌋ = %g", input.A, result)
	case "round":
		places := int(input.B)
		if places < 0 {
			places = 0
		}
		if places > 10 {
			places = 10
		}
		factor := math.Pow(10, float64(places))
		result = math.Round(input.A*factor) / factor
		expr = fmt.Sprintf("round(%g, %d) = %g", input.A, places, result)
	case "percentage":
		result = input.A * input.B / 100
		expr = fmt.Sprintf("%g × %g%% = %g", input.A, input.B, result)
	default:
		return CalculatorOutput{}, fmt.Errorf("unknown operation %q; use add, subtract, multiply, divide, ceil_divide, ceil, floor, round, percentage", input.Operation)
	}

	return CalculatorOutput{Result: result, Expression: expr}, nil
}

func evaluateCalculatorExpression(raw string) (float64, error) {
	if len(raw) > maxCalculatorExprLength {
		return 0, fmt.Errorf("expression exceeds maximum length of %d characters", maxCalculatorExprLength)
	}
	expr, err := parser.ParseExpr(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %w", err)
	}

	result, err := evaluateCalculatorAST(expr, 0)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("expression result must be finite")
	}
	return result, nil
}

func evaluateCalculatorAST(expr ast.Expr, depth int) (float64, error) {
	if depth > maxCalculatorDepth {
		return 0, fmt.Errorf("expression nesting exceeds maximum depth of %d", maxCalculatorDepth)
	}
	switch node := expr.(type) {
	case *ast.BasicLit:
		return parseCalculatorLiteral(node)
	case *ast.ParenExpr:
		return evaluateCalculatorAST(node.X, depth+1)
	case *ast.UnaryExpr:
		return evaluateCalculatorUnary(node, depth+1)
	case *ast.BinaryExpr:
		return evaluateCalculatorBinary(node, depth+1)
	case *ast.CallExpr:
		return evaluateCalculatorCall(node, depth+1)
	default:
		return 0, fmt.Errorf("unsupported expression element %T", expr)
	}
}

func parseCalculatorLiteral(node *ast.BasicLit) (float64, error) {
	if node.Kind != token.INT && node.Kind != token.FLOAT {
		return 0, fmt.Errorf("unsupported literal %q", node.Value)
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric literal %q", node.Value)
	}
	return value, nil
}

func evaluateCalculatorUnary(node *ast.UnaryExpr, depth int) (float64, error) {
	value, err := evaluateCalculatorAST(node.X, depth)
	if err != nil {
		return 0, err
	}
	if node.Op == token.ADD {
		return value, nil
	}
	if node.Op == token.SUB {
		return -value, nil
	}
	return 0, fmt.Errorf("unsupported unary operator %q", node.Op)
}

func evaluateCalculatorBinary(node *ast.BinaryExpr, depth int) (float64, error) {
	left, err := evaluateCalculatorAST(node.X, depth)
	if err != nil {
		return 0, err
	}
	right, err := evaluateCalculatorAST(node.Y, depth)
	if err != nil {
		return 0, err
	}

	switch node.Op {
	case token.ADD:
		return left + right, nil
	case token.SUB:
		return left - right, nil
	case token.MUL:
		return left * right, nil
	case token.QUO:
		if right == 0 {
			return 0, errors.New(divisionByZeroMessage)
		}
		return left / right, nil
	default:
		return 0, fmt.Errorf("unsupported operator %q", node.Op)
	}
}

func evaluateCalculatorCall(node *ast.CallExpr, depth int) (float64, error) {
	name, ok := node.Fun.(*ast.Ident)
	if !ok {
		return 0, fmt.Errorf("unsupported function call")
	}

	args, err := evaluateCalculatorArgs(node.Args, depth)
	if err != nil {
		return 0, err
	}

	return evaluateCalculatorFunction(name.Name, args)
}

func evaluateCalculatorArgs(expressions []ast.Expr, depth int) ([]float64, error) {
	args := make([]float64, 0, len(expressions))
	for _, expression := range expressions {
		value, err := evaluateCalculatorAST(expression, depth)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	return args, nil
}

func evaluateCalculatorFunction(name string, args []float64) (float64, error) {
	handler, ok := map[string]func([]float64) (float64, error){
		"ceil":        calculatorCeil,
		"floor":       calculatorFloor,
		"round":       calculatorRound,
		"percentage":  calculatorPercentage,
		"ceil_divide": calculatorCeilDivide,
	}[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return 0, fmt.Errorf("unsupported function %q; use ceil, floor, round, percentage, ceil_divide", name)
	}
	return handler(args)
}

func calculatorCeil(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("ceil", args, 1); err != nil {
		return 0, err
	}
	return math.Ceil(args[0]), nil
}

func calculatorFloor(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("floor", args, 1); err != nil {
		return 0, err
	}
	return math.Floor(args[0]), nil
}

func calculatorRound(args []float64) (float64, error) {
	if len(args) == 1 {
		return math.Round(args[0]), nil
	}
	if err := requireCalculatorArgCountRange("round", args, 1, 2); err != nil {
		return 0, err
	}
	places := args[1]
	if places != math.Trunc(places) {
		return 0, fmt.Errorf("round decimal places must be an integer")
	}
	if places < 0 {
		places = 0
	}
	if places > 10 {
		places = 10
	}
	factor := math.Pow(10, places)
	return math.Round(args[0]*factor) / factor, nil
}

func calculatorPercentage(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("percentage", args, 2); err != nil {
		return 0, err
	}
	return args[0] * args[1] / 100, nil
}

func calculatorCeilDivide(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("ceil_divide", args, 2); err != nil {
		return 0, err
	}
	if args[1] == 0 {
		return 0, errors.New(divisionByZeroMessage)
	}
	return math.Ceil(args[0] / args[1]), nil
}

func requireCalculatorArgCount(name string, args []float64, want int) error {
	if len(args) != want {
		return fmt.Errorf("%s expects %d argument", name, want)
	}
	return nil
}

func requireCalculatorArgCountRange(name string, args []float64, min int, max int) error {
	if len(args) < min || len(args) > max {
		return fmt.Errorf("%s expects %d or %d arguments", name, min, max)
	}
	return nil
}

func createCalculatorTool() (tool.Tool, error) {
	return apptools.NewCalculatorTool(`Performs exact arithmetic. You MUST use this for ANY calculation and never do math in your head.
Preferred input:
	expression      -> one full arithmetic expression using +, -, *, /, parentheses,
										and helper functions ceil(...), floor(...), round(...), percentage(...), ceil_divide(...)
Legacy input remains supported:
	operation + a/b -> add, subtract, multiply, divide, ceil_divide, ceil, floor, round, percentage
Examples:
	Window area 2m x 1.5m: Calculator(expression="2 * 1.5") -> 3
	Sheets needed: Calculator(expression="ceil_divide(4, 2.5)") -> 2
	Material subtotal plus VAT: Calculator(expression="((15.99 * 3) + (12.50 * 2)) * 1.21") -> 88.2937
	Material subtotal plus VAT plus 10%% markup: Calculator(expression="(((15.99 * 3) + (12.50 * 2)) * 1.21) * 1.10") -> 97.12307`, handleCalculator)
}

// MaxSafeUnitPrice is the ceiling for a single material item (€5M).
const MaxSafeUnitPrice = 5_000_000.00

func createCalculateEstimateTool() (tool.Tool, error) {
	return apptools.NewCalculateEstimateTool(func(ctx tool.Context, input CalculateEstimateInput) (CalculateEstimateOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return CalculateEstimateOutput{}, err
		}
		if err := validateCalculateEstimateInput(input); err != nil {
			return CalculateEstimateOutput{}, err
		}

		materialCents := calculateMaterialSubtotalCents(input)
		laborLowCents, laborHighCents := calculateLaborSubtotalRangeCents(input)
		extraCents := int64(math.Round(input.ExtraCosts * 100))
		deps.SetLastEstimateSnapshot(EstimateComputationSnapshot{
			MaterialSubtotalCents:  materialCents,
			LaborSubtotalLowCents:  laborLowCents,
			LaborSubtotalHighCents: laborHighCents,
			TotalLowCents:          materialCents + laborLowCents + extraCents,
			TotalHighCents:         materialCents + laborHighCents + extraCents,
			ExtraCostsCents:        extraCents,
		})

		return buildCalculateEstimateOutput(materialCents, laborLowCents, laborHighCents, extraCents), nil
	})
}

func isInvalidFloat(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}

func validateCalculateEstimateInput(input CalculateEstimateInput) error {
	// Reject NaN/Inf inputs immediately — they bypass all comparative checks.
	if isInvalidFloat(input.LaborHoursLow) || isInvalidFloat(input.LaborHoursHigh) ||
		isInvalidFloat(input.HourlyRateLow) || isInvalidFloat(input.HourlyRateHigh) ||
		isInvalidFloat(input.ExtraCosts) {
		return fmt.Errorf("financial inputs must be valid finite numbers")
	}

	// Reject negative financial inputs (AI hallucination guard).
	if input.LaborHoursLow < 0 || input.LaborHoursHigh < 0 ||
		input.HourlyRateLow < 0 || input.HourlyRateHigh < 0 ||
		input.ExtraCosts < 0 {
		return fmt.Errorf("financial inputs cannot be negative")
	}

	// Prevent float64->int64 overflow in labor subtotals.
	const maxSafeLaborValue = 1_000_000.0
	if input.LaborHoursLow > maxSafeLaborValue || input.LaborHoursHigh > maxSafeLaborValue {
		return fmt.Errorf("labor hours exceed safety limit of %.2f", maxSafeLaborValue)
	}
	if input.HourlyRateLow > maxSafeLaborValue || input.HourlyRateHigh > maxSafeLaborValue {
		return fmt.Errorf("hourly rate exceeds safety limit of %.2f", maxSafeLaborValue)
	}

	// Prevent float64->int64 overflow in material subtotals.
	const maxSafeQuantity = 1_000_000_000.0
	const maxSafeExtraCosts = 5_000_000.0

	for _, item := range input.MaterialItems {
		if isInvalidFloat(item.UnitPrice) || isInvalidFloat(item.Quantity) {
			return fmt.Errorf("material item price and quantity must be valid finite numbers")
		}
		if item.UnitPrice < 0 || item.Quantity < 0 {
			return fmt.Errorf("material item price and quantity cannot be negative")
		}
		if item.UnitPrice > MaxSafeUnitPrice {
			return fmt.Errorf("unitPrice %.2f exceeds safety limit of %.2f", item.UnitPrice, MaxSafeUnitPrice)
		}
		if item.Quantity > maxSafeQuantity {
			return fmt.Errorf("quantity %.2f exceeds safety limit of %.2f", item.Quantity, maxSafeQuantity)
		}
	}

	if input.ExtraCosts > maxSafeExtraCosts {
		return fmt.Errorf("extra costs %.2f exceed safety limit of %.2f", input.ExtraCosts, maxSafeExtraCosts)
	}

	return nil
}

func calculateMaterialSubtotalCents(input CalculateEstimateInput) int64 {
	// Calculate using integer cents to avoid IEEE 754 precision loss.
	var materialCents int64
	for _, item := range input.MaterialItems {
		if item.UnitPrice <= 0 || item.Quantity <= 0 {
			continue
		}
		unitCents := int64(math.Round(item.UnitPrice * 100))
		lineCents := int64(math.Round(float64(unitCents) * item.Quantity))
		materialCents += lineCents
	}
	return materialCents
}

func calculateLaborSubtotalRangeCents(input CalculateEstimateInput) (int64, int64) {
	laborLowCents := int64(math.Round(input.LaborHoursLow * input.HourlyRateLow * 100))
	laborHighCents := int64(math.Round(input.LaborHoursHigh * input.HourlyRateHigh * 100))
	if laborHighCents < laborLowCents {
		laborLowCents, laborHighCents = laborHighCents, laborLowCents
	}
	return laborLowCents, laborHighCents
}

func buildCalculateEstimateOutput(materialCents, laborLowCents, laborHighCents, extraCents int64) CalculateEstimateOutput {
	return CalculateEstimateOutput{
		MaterialSubtotal:  centsToEuro(materialCents),
		LaborSubtotalLow:  centsToEuro(laborLowCents),
		LaborSubtotalHigh: centsToEuro(laborHighCents),
		TotalLow:          centsToEuro(materialCents + laborLowCents + extraCents),
		TotalHigh:         centsToEuro(materialCents + laborHighCents + extraCents),
		AppliedExtraCosts: centsToEuro(extraCents),
	}
}

func centsToEuro(cents int64) float64 {
	return math.Round(float64(cents)) / 100.0
}

// defaultSearchScoreThreshold is the minimum cosine similarity score for
// BGE-M3 embeddings. It controls recall (what enters candidate set).
const defaultSearchScoreThreshold = 0.35
const maxCatalogRewordRetries = 2
