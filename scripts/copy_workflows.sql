-- Copy complete workflows from source org to all other orgs
-- Source org: Timmerbedrijf Henk Oost (3578b0f5-727a-46b2-8d1e-d7b9820587de)

DO $$
DECLARE
    source_org_id UUID := '3578b0f5-727a-46b2-8d1e-d7b9820587de';
    target_org RECORD;
    old_wf_id UUID;
    new_wf_id UUID;
    old_rule_id UUID;
    default_wf_id UUID;
BEGIN
    -- Loop over every org except the source
    FOR target_org IN
        SELECT id FROM RAC_organizations WHERE id != source_org_id
    LOOP
        RAISE NOTICE 'Processing org %', target_org.id;

        -- 1. Delete existing assignment rules for target org
        DELETE FROM RAC_workflow_assignment_rules WHERE organization_id = target_org.id;

        -- 2. Delete existing workflow steps for target org
        DELETE FROM RAC_workflow_steps WHERE organization_id = target_org.id;

        -- 3. Delete existing workflows for target org
        DELETE FROM RAC_workflows WHERE organization_id = target_org.id;

        -- 4. Copy workflows from source to target, remembering old->new ID mapping
        -- We use a temp table to map old workflow IDs to new ones
        CREATE TEMP TABLE IF NOT EXISTS _wf_id_map (old_id UUID, new_id UUID, target_org_id UUID);
        DELETE FROM _wf_id_map WHERE target_org_id = target_org.id;

        FOR old_wf_id IN
            SELECT id FROM RAC_workflows WHERE organization_id = source_org_id
        LOOP
            new_wf_id := gen_random_uuid();
            INSERT INTO _wf_id_map (old_id, new_id, target_org_id) VALUES (old_wf_id, new_wf_id, target_org.id);

            INSERT INTO RAC_workflows (
                id, organization_id, workflow_key, name, description,
                enabled, quote_valid_days_override, quote_payment_days_override
            )
            SELECT
                new_wf_id, target_org.id, workflow_key, name, description,
                enabled, quote_valid_days_override, quote_payment_days_override
            FROM RAC_workflows
            WHERE id = old_wf_id;
        END LOOP;

        -- 5. Copy workflow steps using the ID map
        INSERT INTO RAC_workflow_steps (
            id, organization_id, workflow_id, trigger, channel, audience,
            action, step_order, delay_minutes, enabled, recipient_config,
            template_subject, template_body, stop_on_reply
        )
        SELECT
            gen_random_uuid(),
            target_org.id,
            m.new_id,
            s.trigger,
            s.channel,
            s.audience,
            s.action,
            s.step_order,
            s.delay_minutes,
            s.enabled,
            s.recipient_config,
            s.template_subject,
            s.template_body,
            s.stop_on_reply
        FROM RAC_workflow_steps s
        JOIN _wf_id_map m ON m.old_id = s.workflow_id AND m.target_org_id = target_org.id
        WHERE s.organization_id = source_org_id;

        -- 6. Copy assignment rules, pointing to the new default workflow
        -- Find which workflow was the "default" in the source (lowest workflow_key)
        SELECT m.new_id INTO default_wf_id
        FROM _wf_id_map m
        JOIN RAC_workflows w ON w.id = m.old_id
        WHERE m.target_org_id = target_org.id
        ORDER BY w.workflow_key
        LIMIT 1;

        IF default_wf_id IS NOT NULL THEN
            INSERT INTO RAC_workflow_assignment_rules (
                id, organization_id, workflow_id, name, enabled, priority,
                lead_source, lead_service_type, pipeline_stage
            )
            SELECT
                gen_random_uuid(),
                target_org.id,
                default_wf_id,
                'Default workflow',
                true,
                1000000,
                NULL,
                NULL,
                NULL;
        END IF;

        RAISE NOTICE 'Done with org % — created workflows + steps + rule', target_org.id;
    END LOOP;

    -- Cleanup temp table
    DROP TABLE IF EXISTS _wf_id_map;

END $$;
