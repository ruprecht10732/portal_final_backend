package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	identitydb "portal_final_backend/internal/identity/db"
)

type Workflow struct {
	ID                       uuid.UUID
	OrganizationID           uuid.UUID
	WorkflowKey              string
	Name                     string
	Description              *string
	Enabled                  bool
	QuoteValidDaysOverride   *int
	QuotePaymentDaysOverride *int
	CreatedAt                time.Time
	UpdatedAt                time.Time
	Steps                    []WorkflowStep
}

type WorkflowStep struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	WorkflowID      uuid.UUID
	Trigger         string
	Channel         string
	Audience        string
	Action          string
	StepOrder       int
	DelayMinutes    int
	Enabled         bool
	RecipientConfig map[string]any
	TemplateSubject *string
	TemplateBody    *string
	StopOnReply     bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkflowUpsert struct {
	ID                       *uuid.UUID
	WorkflowKey              string
	Name                     string
	Description              *string
	Enabled                  bool
	QuoteValidDaysOverride   *int
	QuotePaymentDaysOverride *int
	Steps                    []WorkflowStepUpsert
}

type WorkflowStepUpsert struct {
	ID              *uuid.UUID
	Trigger         string
	Channel         string
	Audience        string
	Action          string
	StepOrder       int
	DelayMinutes    int
	Enabled         bool
	RecipientConfig map[string]any
	TemplateSubject *string
	TemplateBody    *string
	StopOnReply     bool
}

type WorkflowAssignmentRule struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	WorkflowID      uuid.UUID
	Name            string
	Enabled         bool
	Priority        int
	LeadSource      *string
	LeadServiceType *string
	PipelineStage   *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkflowAssignmentRuleUpsert struct {
	ID              *uuid.UUID
	WorkflowID      uuid.UUID
	Name            string
	Enabled         bool
	Priority        int
	LeadSource      *string
	LeadServiceType *string
	PipelineStage   *string
}

type LeadWorkflowOverride struct {
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	WorkflowID     *uuid.UUID
	OverrideMode   string
	Reason         *string
	AssignedBy     *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type LeadWorkflowOverrideUpsert struct {
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	WorkflowID     *uuid.UUID
	OverrideMode   string
	Reason         *string
	AssignedBy     *uuid.UUID
}

func (r *Repository) ListWorkflows(ctx context.Context, organizationID uuid.UUID) ([]Workflow, error) {
	workflowRows, err := r.queries.ListWorkflows(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}

	workflows := make([]Workflow, 0, len(workflowRows))
	workflowByID := make(map[uuid.UUID]int, len(workflowRows))
	for _, row := range workflowRows {
		workflow := workflowFromModel(row)
		workflowByID[workflow.ID] = len(workflows)
		workflows = append(workflows, workflow)
	}
	if len(workflows) == 0 {
		return workflows, nil
	}

	stepRows, err := r.queries.ListWorkflowSteps(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}
	for _, row := range stepRows {
		step, err := workflowStepFromModel(row)
		if err != nil {
			return nil, err
		}
		if idx, ok := workflowByID[step.WorkflowID]; ok {
			workflows[idx].Steps = append(workflows[idx].Steps, step)
		}
	}

	return workflows, nil
}

func (r *Repository) ReplaceWorkflows(ctx context.Context, organizationID uuid.UUID, workflows []WorkflowUpsert) ([]Workflow, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	result := make([]Workflow, 0, len(workflows))
	keptWorkflowIDs := make([]uuid.UUID, 0, len(workflows))
	for _, workflow := range workflows {
		wf, err := upsertWorkflowTx(ctx, queries, organizationID, workflow)
		if err != nil {
			return nil, err
		}
		keptWorkflowIDs = append(keptWorkflowIDs, wf.ID)

		steps, err := upsertWorkflowStepsTx(ctx, queries, organizationID, wf.ID, workflow.Steps)
		if err != nil {
			return nil, err
		}
		wf.Steps = steps
		result = append(result, wf)
	}

	if len(keptWorkflowIDs) == 0 {
		if err := queries.DeleteWorkflowsByOrganization(ctx, toPgUUID(organizationID)); err != nil {
			return nil, err
		}
	} else {
		if err := queries.DeleteWorkflowsNotInList(ctx, identitydb.DeleteWorkflowsNotInListParams{
			OrganizationID: toPgUUID(organizationID),
			Column2:        toPgUUIDSlice(keptWorkflowIDs),
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) EnsureDefaultWorkflowSeed(ctx context.Context, organizationID uuid.UUID, workflow WorkflowUpsert, defaultRuleName string, defaultPriority int) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	wf, err := upsertWorkflowTx(ctx, queries, organizationID, workflow)
	if err != nil {
		return err
	}
	if _, err := upsertWorkflowStepsTx(ctx, queries, organizationID, wf.ID, workflow.Steps); err != nil {
		return err
	}

	defaultRuleExists, err := queries.DefaultWorkflowAssignmentRuleExists(ctx, toPgUUID(organizationID))
	if err != nil {
		return err
	}
	if !defaultRuleExists {
		if err := queries.CreateDefaultWorkflowAssignmentRule(ctx, identitydb.CreateDefaultWorkflowAssignmentRuleParams{
			OrganizationID: toPgUUID(organizationID),
			WorkflowID:     toPgUUID(wf.ID),
			Name:           defaultRuleName,
			Priority:       int32(defaultPriority),
		}); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func upsertWorkflowTx(ctx context.Context, queries *identitydb.Queries, organizationID uuid.UUID, workflow WorkflowUpsert) (Workflow, error) {
	workflowID := uuid.New()
	if workflow.ID != nil && *workflow.ID != uuid.Nil {
		workflowID = *workflow.ID
	}

	_, err := queries.UpsertWorkflow(ctx, identitydb.UpsertWorkflowParams{
		ID:                       toPgUUID(workflowID),
		OrganizationID:           toPgUUID(organizationID),
		WorkflowKey:              workflow.WorkflowKey,
		Name:                     workflow.Name,
		Description:              toPgTextPtr(workflow.Description),
		Enabled:                  workflow.Enabled,
		QuoteValidDaysOverride:   toPgInt4Ptr(workflow.QuoteValidDaysOverride),
		QuotePaymentDaysOverride: toPgInt4Ptr(workflow.QuotePaymentDaysOverride),
	})
	if err != nil {
		return Workflow{}, err
	}
	now := time.Now()
	return Workflow{
		ID:                       workflowID,
		OrganizationID:           organizationID,
		WorkflowKey:              workflow.WorkflowKey,
		Name:                     workflow.Name,
		Description:              workflow.Description,
		Enabled:                  workflow.Enabled,
		QuoteValidDaysOverride:   workflow.QuoteValidDaysOverride,
		QuotePaymentDaysOverride: workflow.QuotePaymentDaysOverride,
		CreatedAt:                now,
		UpdatedAt:                now,
	}, nil
}

func upsertWorkflowStepsTx(ctx context.Context, queries *identitydb.Queries, organizationID uuid.UUID, workflowID uuid.UUID, steps []WorkflowStepUpsert) ([]WorkflowStep, error) {
	keptStepIDs := make([]uuid.UUID, 0, len(steps))
	result := make([]WorkflowStep, 0, len(steps))
	now := time.Now()
	for _, step := range steps {
		stepID := uuid.New()
		if step.ID != nil && *step.ID != uuid.Nil {
			stepID = *step.ID
		}

		recipientConfigJSON, err := marshalRecipientConfig(step.RecipientConfig)
		if err != nil {
			return nil, err
		}

		_, err = queries.UpsertWorkflowStep(ctx, identitydb.UpsertWorkflowStepParams{
			ID:              toPgUUID(stepID),
			OrganizationID:  toPgUUID(organizationID),
			WorkflowID:      toPgUUID(workflowID),
			Trigger:         step.Trigger,
			Channel:         step.Channel,
			Audience:        step.Audience,
			Action:          step.Action,
			StepOrder:       int32(step.StepOrder),
			DelayMinutes:    int32(step.DelayMinutes),
			Enabled:         step.Enabled,
			Column11:        recipientConfigJSON,
			TemplateSubject: toPgTextPtr(step.TemplateSubject),
			TemplateBody:    toPgTextPtr(step.TemplateBody),
			StopOnReply:     step.StopOnReply,
		})
		if err != nil {
			return nil, err
		}
		keptStepIDs = append(keptStepIDs, stepID)
		result = append(result, WorkflowStep{
			ID:              stepID,
			OrganizationID:  organizationID,
			WorkflowID:      workflowID,
			Trigger:         step.Trigger,
			Channel:         step.Channel,
			Audience:        step.Audience,
			Action:          step.Action,
			StepOrder:       step.StepOrder,
			DelayMinutes:    step.DelayMinutes,
			Enabled:         step.Enabled,
			RecipientConfig: step.RecipientConfig,
			TemplateSubject: step.TemplateSubject,
			TemplateBody:    step.TemplateBody,
			StopOnReply:     step.StopOnReply,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}

	if len(keptStepIDs) == 0 {
		if err := queries.DeleteWorkflowStepsByWorkflow(ctx, identitydb.DeleteWorkflowStepsByWorkflowParams{
			OrganizationID: toPgUUID(organizationID),
			WorkflowID:     toPgUUID(workflowID),
		}); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := queries.DeleteWorkflowStepsNotInList(ctx, identitydb.DeleteWorkflowStepsNotInListParams{
		OrganizationID: toPgUUID(organizationID),
		WorkflowID:     toPgUUID(workflowID),
		Column3:        toPgUUIDSlice(keptStepIDs),
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func marshalRecipientConfig(config map[string]any) ([]byte, error) {
	if config == nil {
		config = map[string]any{}
	}
	return json.Marshal(config)
}

func (r *Repository) ListWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID) ([]WorkflowAssignmentRule, error) {
	rows, err := r.queries.ListWorkflowAssignmentRules(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}
	result := make([]WorkflowAssignmentRule, 0, len(rows))
	for _, row := range rows {
		result = append(result, workflowAssignmentRuleFromModel(row))
	}
	return result, nil
}

func (r *Repository) ReplaceWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID, rules []WorkflowAssignmentRuleUpsert) ([]WorkflowAssignmentRule, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	if err := queries.DeleteWorkflowAssignmentRulesByOrganization(ctx, toPgUUID(organizationID)); err != nil {
		return nil, err
	}

	for _, rule := range rules {
		ruleID := uuid.New()
		if rule.ID != nil && *rule.ID != uuid.Nil {
			ruleID = *rule.ID
		}
		if err := queries.CreateWorkflowAssignmentRule(ctx, identitydb.CreateWorkflowAssignmentRuleParams{
			ID:              toPgUUID(ruleID),
			OrganizationID:  toPgUUID(organizationID),
			WorkflowID:      toPgUUID(rule.WorkflowID),
			Name:            rule.Name,
			Enabled:         rule.Enabled,
			Priority:        int32(rule.Priority),
			LeadSource:      toPgTextPtr(rule.LeadSource),
			LeadServiceType: toPgTextPtr(rule.LeadServiceType),
			PipelineStage:   toPgTextPtr(rule.PipelineStage),
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListWorkflowAssignmentRules(ctx, organizationID)
}

func (r *Repository) GetLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadWorkflowOverride, error) {
	row, err := r.queries.GetLeadWorkflowOverride(ctx, identitydb.GetLeadWorkflowOverrideParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadWorkflowOverride{}, ErrNotFound
	}
	if err != nil {
		return LeadWorkflowOverride{}, err
	}
	return leadWorkflowOverrideFromModel(row), nil
}

func (r *Repository) UpsertLeadWorkflowOverride(ctx context.Context, upsert LeadWorkflowOverrideUpsert) (LeadWorkflowOverride, error) {
	row, err := r.queries.UpsertLeadWorkflowOverride(ctx, identitydb.UpsertLeadWorkflowOverrideParams{
		LeadID:         toPgUUID(upsert.LeadID),
		OrganizationID: toPgUUID(upsert.OrganizationID),
		WorkflowID:     toPgUUIDPtr(upsert.WorkflowID),
		OverrideMode:   upsert.OverrideMode,
		Reason:         toPgTextPtr(upsert.Reason),
		AssignedBy:     toPgUUIDPtr(upsert.AssignedBy),
	})
	if err != nil {
		return LeadWorkflowOverride{}, err
	}
	return leadWorkflowOverrideFromModel(row), nil
}

func (r *Repository) DeleteLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error {
	return r.queries.DeleteLeadWorkflowOverride(ctx, identitydb.DeleteLeadWorkflowOverrideParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
}

func (r *Repository) LeadExistsInOrganization(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (bool, error) {
	return r.queries.LeadExistsInOrganization(ctx, identitydb.LeadExistsInOrganizationParams{
		ID:             toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
}

func (r *Repository) WorkflowExistsInOrganization(ctx context.Context, workflowID uuid.UUID, organizationID uuid.UUID) (bool, error) {
	return r.queries.WorkflowExistsInOrganization(ctx, identitydb.WorkflowExistsInOrganizationParams{
		ID:             toPgUUID(workflowID),
		OrganizationID: toPgUUID(organizationID),
	})
}

func workflowFromModel(row identitydb.RacWorkflow) Workflow {
	return Workflow{
		ID:                       uuidFromPg(row.ID),
		OrganizationID:           uuidFromPg(row.OrganizationID),
		WorkflowKey:              row.WorkflowKey,
		Name:                     row.Name,
		Description:              optionalString(row.Description),
		Enabled:                  row.Enabled,
		QuoteValidDaysOverride:   optionalInt(row.QuoteValidDaysOverride),
		QuotePaymentDaysOverride: optionalInt(row.QuotePaymentDaysOverride),
		CreatedAt:                timeFromPg(row.CreatedAt),
		UpdatedAt:                timeFromPg(row.UpdatedAt),
	}
}

func workflowStepFromModel(row identitydb.RacWorkflowStep) (WorkflowStep, error) {
	step := WorkflowStep{
		ID:              uuidFromPg(row.ID),
		OrganizationID:  uuidFromPg(row.OrganizationID),
		WorkflowID:      uuidFromPg(row.WorkflowID),
		Trigger:         row.Trigger,
		Channel:         row.Channel,
		Audience:        row.Audience,
		Action:          row.Action,
		StepOrder:       int(row.StepOrder),
		DelayMinutes:    int(row.DelayMinutes),
		Enabled:         row.Enabled,
		TemplateSubject: optionalString(row.TemplateSubject),
		TemplateBody:    optionalString(row.TemplateBody),
		StopOnReply:     row.StopOnReply,
		CreatedAt:       timeFromPg(row.CreatedAt),
		UpdatedAt:       timeFromPg(row.UpdatedAt),
	}
	if len(row.RecipientConfig) > 0 {
		if err := json.Unmarshal(row.RecipientConfig, &step.RecipientConfig); err != nil {
			return WorkflowStep{}, err
		}
	}
	if step.RecipientConfig == nil {
		step.RecipientConfig = map[string]any{}
	}
	return step, nil
}

func workflowAssignmentRuleFromModel(row identitydb.RacWorkflowAssignmentRule) WorkflowAssignmentRule {
	return WorkflowAssignmentRule{
		ID:              uuidFromPg(row.ID),
		OrganizationID:  uuidFromPg(row.OrganizationID),
		WorkflowID:      uuidFromPg(row.WorkflowID),
		Name:            row.Name,
		Enabled:         row.Enabled,
		Priority:        int(row.Priority),
		LeadSource:      optionalString(row.LeadSource),
		LeadServiceType: optionalString(row.LeadServiceType),
		PipelineStage:   optionalString(row.PipelineStage),
		CreatedAt:       timeFromPg(row.CreatedAt),
		UpdatedAt:       timeFromPg(row.UpdatedAt),
	}
}

func leadWorkflowOverrideFromModel(row identitydb.RacLeadWorkflowOverride) LeadWorkflowOverride {
	return LeadWorkflowOverride{
		LeadID:         uuidFromPg(row.LeadID),
		OrganizationID: uuidFromPg(row.OrganizationID),
		WorkflowID:     optionalUUID(row.WorkflowID),
		OverrideMode:   row.OverrideMode,
		Reason:         optionalString(row.Reason),
		AssignedBy:     optionalUUID(row.AssignedBy),
		CreatedAt:      timeFromPg(row.CreatedAt),
		UpdatedAt:      timeFromPg(row.UpdatedAt),
	}
}

func toPgUUIDSlice(values []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		result = append(result, toPgUUID(value))
	}
	return result
}
