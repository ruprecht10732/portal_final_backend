package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	workflows, workflowByID, err := r.fetchWorkflows(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if len(workflows) == 0 {
		return workflows, nil
	}

	if err := r.attachWorkflowSteps(ctx, organizationID, workflows, workflowByID); err != nil {
		return nil, err
	}

	return workflows, nil
}

func (r *Repository) ReplaceWorkflows(ctx context.Context, organizationID uuid.UUID, workflows []WorkflowUpsert) ([]Workflow, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.replaceWorkflowsTx(ctx, tx, organizationID, workflows); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return r.ListWorkflows(ctx, organizationID)
}

func (r *Repository) EnsureDefaultWorkflowSeed(
	ctx context.Context,
	organizationID uuid.UUID,
	workflow WorkflowUpsert,
	defaultRuleName string,
	defaultPriority int,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	workflowID, err := upsertWorkflowTx(ctx, tx, organizationID, workflow)
	if err != nil {
		return err
	}

	if err := upsertWorkflowStepsTx(ctx, tx, organizationID, workflowID, workflow.Steps); err != nil {
		return err
	}

	var defaultRuleExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM RAC_workflow_assignment_rules
			WHERE organization_id = $1
			  AND lead_source IS NULL
			  AND lead_service_type IS NULL
			  AND pipeline_stage IS NULL
		)
	`, organizationID).Scan(&defaultRuleExists)
	if err != nil {
		return err
	}

	if !defaultRuleExists {
		if _, err := tx.Exec(ctx, `
			INSERT INTO RAC_workflow_assignment_rules (
				organization_id,
				workflow_id,
				name,
				enabled,
				priority,
				lead_source,
				lead_service_type,
				pipeline_stage
			)
			VALUES ($1, $2, $3, TRUE, $4, NULL, NULL, NULL)
		`, organizationID, workflowID, defaultRuleName, defaultPriority); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Repository) fetchWorkflows(ctx context.Context, organizationID uuid.UUID) ([]Workflow, map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, workflow_key, name, description, enabled,
		       quote_valid_days_override, quote_payment_days_override, created_at, updated_at
		FROM RAC_workflows
		WHERE organization_id = $1
		ORDER BY workflow_key ASC
	`, organizationID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	result := make([]Workflow, 0)
	workflowByID := make(map[uuid.UUID]int)
	for rows.Next() {
		var workflow Workflow
		if err := rows.Scan(
			&workflow.ID,
			&workflow.OrganizationID,
			&workflow.WorkflowKey,
			&workflow.Name,
			&workflow.Description,
			&workflow.Enabled,
			&workflow.QuoteValidDaysOverride,
			&workflow.QuotePaymentDaysOverride,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		workflowByID[workflow.ID] = len(result)
		result = append(result, workflow)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return result, workflowByID, nil
}

func (r *Repository) attachWorkflowSteps(ctx context.Context, organizationID uuid.UUID, workflows []Workflow, workflowByID map[uuid.UUID]int) error {
	stepRows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, workflow_id, trigger, channel, audience, action,
		       step_order, delay_minutes, enabled, recipient_config, template_subject,
		       template_body, stop_on_reply, created_at, updated_at
		FROM RAC_workflow_steps
		WHERE organization_id = $1
		ORDER BY workflow_id ASC, trigger ASC, channel ASC, step_order ASC
	`, organizationID)
	if err != nil {
		return err
	}
	defer stepRows.Close()

	for stepRows.Next() {
		step, err := scanWorkflowStep(stepRows)
		if err != nil {
			return err
		}
		if idx, ok := workflowByID[step.WorkflowID]; ok {
			workflows[idx].Steps = append(workflows[idx].Steps, step)
		}
	}

	return stepRows.Err()
}

func scanWorkflowStep(rows pgx.Rows) (WorkflowStep, error) {
	var step WorkflowStep
	var rawRecipientConfig []byte
	if err := rows.Scan(
		&step.ID,
		&step.OrganizationID,
		&step.WorkflowID,
		&step.Trigger,
		&step.Channel,
		&step.Audience,
		&step.Action,
		&step.StepOrder,
		&step.DelayMinutes,
		&step.Enabled,
		&rawRecipientConfig,
		&step.TemplateSubject,
		&step.TemplateBody,
		&step.StopOnReply,
		&step.CreatedAt,
		&step.UpdatedAt,
	); err != nil {
		return WorkflowStep{}, err
	}

	if len(rawRecipientConfig) > 0 {
		if err := json.Unmarshal(rawRecipientConfig, &step.RecipientConfig); err != nil {
			return WorkflowStep{}, err
		}
	}
	if step.RecipientConfig == nil {
		step.RecipientConfig = map[string]any{}
	}

	return step, nil
}

func (r *Repository) replaceWorkflowsTx(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, workflows []WorkflowUpsert) error {
	keptWorkflowIDs := make([]uuid.UUID, 0, len(workflows))

	for _, workflow := range workflows {
		workflowID, err := upsertWorkflowTx(ctx, tx, organizationID, workflow)
		if err != nil {
			return err
		}
		keptWorkflowIDs = append(keptWorkflowIDs, workflowID)

		if err := upsertWorkflowStepsTx(ctx, tx, organizationID, workflowID, workflow.Steps); err != nil {
			return err
		}
	}

	if len(keptWorkflowIDs) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM RAC_workflows WHERE organization_id = $1`, organizationID); err != nil {
			return err
		}
		return nil
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM RAC_workflows
		WHERE organization_id = $1
		  AND NOT (id = ANY($2::uuid[]))
	`, organizationID, keptWorkflowIDs); err != nil {
		return err
	}

	return nil
}

func upsertWorkflowTx(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, workflow WorkflowUpsert) (uuid.UUID, error) {
	workflowID := uuid.New()
	if workflow.ID != nil && *workflow.ID != uuid.Nil {
		workflowID = *workflow.ID
	}

	err := tx.QueryRow(ctx, `
		INSERT INTO RAC_workflows (
			id, organization_id, workflow_key, name, description, enabled,
			quote_valid_days_override, quote_payment_days_override
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, workflow_key) DO UPDATE
		SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			quote_valid_days_override = EXCLUDED.quote_valid_days_override,
			quote_payment_days_override = EXCLUDED.quote_payment_days_override,
			updated_at = now()
		RETURNING id
	`,
		workflowID,
		organizationID,
		workflow.WorkflowKey,
		workflow.Name,
		workflow.Description,
		workflow.Enabled,
		workflow.QuoteValidDaysOverride,
		workflow.QuotePaymentDaysOverride,
	).Scan(&workflowID)
	if err != nil {
		return uuid.Nil, err
	}

	return workflowID, nil
}

func upsertWorkflowStepsTx(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, workflowID uuid.UUID, steps []WorkflowStepUpsert) error {
	keptStepIDs := make([]uuid.UUID, 0, len(steps))

	for _, step := range steps {
		stepID := uuid.New()
		if step.ID != nil && *step.ID != uuid.Nil {
			stepID = *step.ID
		}

		recipientConfigJSON, err := marshalRecipientConfig(step.RecipientConfig)
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO RAC_workflow_steps (
				id, organization_id, workflow_id, trigger, channel, audience, action,
				step_order, delay_minutes, enabled, recipient_config, template_subject,
				template_body, stop_on_reply
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13, $14)
			ON CONFLICT (workflow_id, trigger, channel, step_order) DO UPDATE
			SET
				audience = EXCLUDED.audience,
				action = EXCLUDED.action,
				delay_minutes = EXCLUDED.delay_minutes,
				enabled = EXCLUDED.enabled,
				recipient_config = EXCLUDED.recipient_config,
				template_subject = EXCLUDED.template_subject,
				template_body = EXCLUDED.template_body,
				stop_on_reply = EXCLUDED.stop_on_reply,
				updated_at = now()
			RETURNING id
		`,
			stepID,
			organizationID,
			workflowID,
			step.Trigger,
			step.Channel,
			step.Audience,
			step.Action,
			step.StepOrder,
			step.DelayMinutes,
			step.Enabled,
			recipientConfigJSON,
			step.TemplateSubject,
			step.TemplateBody,
			step.StopOnReply,
		).Scan(&stepID)
		if err != nil {
			return err
		}
		keptStepIDs = append(keptStepIDs, stepID)
	}

	if len(keptStepIDs) == 0 {
		_, err := tx.Exec(ctx, `DELETE FROM RAC_workflow_steps WHERE organization_id = $1 AND workflow_id = $2`, organizationID, workflowID)
		return err
	}

	_, err := tx.Exec(ctx, `
		DELETE FROM RAC_workflow_steps
		WHERE organization_id = $1
		  AND workflow_id = $2
		  AND NOT (id = ANY($3::uuid[]))
	`, organizationID, workflowID, keptStepIDs)
	if err != nil {
		return err
	}

	return nil
}

func marshalRecipientConfig(config map[string]any) ([]byte, error) {
	if config == nil {
		config = map[string]any{}
	}
	return json.Marshal(config)
}

func (r *Repository) ListWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID) ([]WorkflowAssignmentRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, workflow_id, name, enabled, priority,
		       lead_source, lead_service_type, pipeline_stage, created_at, updated_at
		FROM RAC_workflow_assignment_rules
		WHERE organization_id = $1
		ORDER BY priority ASC, created_at ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]WorkflowAssignmentRule, 0)
	for rows.Next() {
		var rule WorkflowAssignmentRule
		if err := rows.Scan(
			&rule.ID,
			&rule.OrganizationID,
			&rule.WorkflowID,
			&rule.Name,
			&rule.Enabled,
			&rule.Priority,
			&rule.LeadSource,
			&rule.LeadServiceType,
			&rule.PipelineStage,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}

	return result, rows.Err()
}

func (r *Repository) ReplaceWorkflowAssignmentRules(ctx context.Context, organizationID uuid.UUID, rules []WorkflowAssignmentRuleUpsert) ([]WorkflowAssignmentRule, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM RAC_workflow_assignment_rules WHERE organization_id = $1`, organizationID); err != nil {
		return nil, err
	}

	for _, rule := range rules {
		ruleID := uuid.New()
		if rule.ID != nil && *rule.ID != uuid.Nil {
			ruleID = *rule.ID
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO RAC_workflow_assignment_rules (
				id, organization_id, workflow_id, name, enabled, priority,
				lead_source, lead_service_type, pipeline_stage
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`,
			ruleID,
			organizationID,
			rule.WorkflowID,
			rule.Name,
			rule.Enabled,
			rule.Priority,
			rule.LeadSource,
			rule.LeadServiceType,
			rule.PipelineStage,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return r.ListWorkflowAssignmentRules(ctx, organizationID)
}

func (r *Repository) GetLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadWorkflowOverride, error) {
	var result LeadWorkflowOverride
	err := r.pool.QueryRow(ctx, `
		SELECT lead_id, organization_id, workflow_id, override_mode, reason, assigned_by, created_at, updated_at
		FROM RAC_lead_workflow_overrides
		WHERE lead_id = $1 AND organization_id = $2
	`, leadID, organizationID).Scan(
		&result.LeadID,
		&result.OrganizationID,
		&result.WorkflowID,
		&result.OverrideMode,
		&result.Reason,
		&result.AssignedBy,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadWorkflowOverride{}, ErrNotFound
	}
	return result, err
}

func (r *Repository) UpsertLeadWorkflowOverride(ctx context.Context, upsert LeadWorkflowOverrideUpsert) (LeadWorkflowOverride, error) {
	var result LeadWorkflowOverride
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_lead_workflow_overrides (
			lead_id, organization_id, workflow_id, override_mode, reason, assigned_by
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (lead_id) DO UPDATE SET
			workflow_id = EXCLUDED.workflow_id,
			override_mode = EXCLUDED.override_mode,
			reason = EXCLUDED.reason,
			assigned_by = EXCLUDED.assigned_by,
			updated_at = now()
		RETURNING lead_id, organization_id, workflow_id, override_mode, reason, assigned_by, created_at, updated_at
	`, upsert.LeadID, upsert.OrganizationID, upsert.WorkflowID, upsert.OverrideMode, upsert.Reason, upsert.AssignedBy).Scan(
		&result.LeadID,
		&result.OrganizationID,
		&result.WorkflowID,
		&result.OverrideMode,
		&result.Reason,
		&result.AssignedBy,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	return result, err
}

func (r *Repository) DeleteLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_lead_workflow_overrides
		WHERE lead_id = $1 AND organization_id = $2
	`, leadID, organizationID)
	return err
}

func (r *Repository) LeadExistsInOrganization(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM RAC_leads
			WHERE id = $1 AND organization_id = $2
		)
	`, leadID, organizationID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) WorkflowExistsInOrganization(ctx context.Context, workflowID uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM RAC_workflows
			WHERE id = $1 AND organization_id = $2
		)
	`, workflowID, organizationID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
