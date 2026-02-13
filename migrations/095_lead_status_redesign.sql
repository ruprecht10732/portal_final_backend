-- +goose Up
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_status_check;
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS rac_leads_status_check;

UPDATE RAC_lead_services
SET status = CASE status
    WHEN 'Scheduled' THEN 'Appointment_Scheduled'
    WHEN 'Surveyed' THEN 'Survey_Completed'
    WHEN 'Bad_Lead' THEN 'Disqualified'
    WHEN 'Closed' THEN 'Completed'
    ELSE status
END;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'RAC_leads'
          AND column_name = 'status'
    ) THEN
        EXECUTE $sql$
            UPDATE RAC_leads
            SET status = CASE status
                WHEN 'Scheduled' THEN 'Appointment_Scheduled'
                WHEN 'Surveyed' THEN 'Survey_Completed'
                WHEN 'Bad_Lead' THEN 'Disqualified'
                WHEN 'Closed' THEN 'Completed'
                ELSE status
            END
        $sql$;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE RAC_lead_services
ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
        'New',
        'Attempted_Contact',
        'Appointment_Scheduled',
        'Survey_Completed',
        'Quote_Draft',
        'Quote_Sent',
        'Quote_Accepted',
        'Partner_Assigned',
        'Needs_Rescheduling',
        'Completed',
        'Lost',
        'Disqualified'
    )
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'RAC_leads'
          AND column_name = 'status'
    ) THEN
        EXECUTE $sql$
            ALTER TABLE RAC_leads
            ADD CONSTRAINT leads_status_check CHECK (
                status IN (
                    'New',
                    'Attempted_Contact',
                    'Appointment_Scheduled',
                    'Survey_Completed',
                    'Quote_Draft',
                    'Quote_Sent',
                    'Quote_Accepted',
                    'Partner_Assigned',
                    'Needs_Rescheduling',
                    'Completed',
                    'Lost',
                    'Disqualified'
                )
            )
        $sql$;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_status_check;
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS rac_leads_status_check;

UPDATE RAC_lead_services
SET status = CASE status
    WHEN 'Appointment_Scheduled' THEN 'Scheduled'
    WHEN 'Survey_Completed' THEN 'Surveyed'
    WHEN 'Disqualified' THEN 'Bad_Lead'
    WHEN 'Completed' THEN 'Closed'
    WHEN 'Quote_Draft' THEN 'New'
    WHEN 'Quote_Sent' THEN 'New'
    WHEN 'Quote_Accepted' THEN 'New'
    WHEN 'Partner_Assigned' THEN 'New'
    WHEN 'Lost' THEN 'Bad_Lead'
    ELSE status
END;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'RAC_leads'
          AND column_name = 'status'
    ) THEN
        EXECUTE $sql$
            UPDATE RAC_leads
            SET status = CASE status
                WHEN 'Appointment_Scheduled' THEN 'Scheduled'
                WHEN 'Survey_Completed' THEN 'Surveyed'
                WHEN 'Disqualified' THEN 'Bad_Lead'
                WHEN 'Completed' THEN 'Closed'
                WHEN 'Quote_Draft' THEN 'New'
                WHEN 'Quote_Sent' THEN 'New'
                WHEN 'Quote_Accepted' THEN 'New'
                WHEN 'Partner_Assigned' THEN 'New'
                WHEN 'Lost' THEN 'Bad_Lead'
                ELSE status
            END
        $sql$;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE RAC_lead_services
ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
        'New',
        'Attempted_Contact',
        'Scheduled',
        'Surveyed',
        'Bad_Lead',
        'Needs_Rescheduling',
        'Closed'
    )
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'RAC_leads'
          AND column_name = 'status'
    ) THEN
        EXECUTE $sql$
            ALTER TABLE RAC_leads
            ADD CONSTRAINT leads_status_check CHECK (
                status IN (
                    'New',
                    'Attempted_Contact',
                    'Scheduled',
                    'Surveyed',
                    'Bad_Lead',
                    'Needs_Rescheduling',
                    'Closed'
                )
            )
        $sql$;
    END IF;
END $$;
-- +goose StatementEnd
