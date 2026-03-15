-- +goose Up

ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_status_check;

ALTER TABLE RAC_lead_services
ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
        'New',
        'Pending',
        'In_Progress',
        'Attempted_Contact',
        'Appointment_Scheduled',
        'Needs_Rescheduling',
        'Completed',
        'Disqualified'
    )
);

UPDATE RAC_lead_services
SET status = 'Completed'
WHERE pipeline_stage = 'Completed'
  AND status <> 'Completed';

-- +goose Down

UPDATE RAC_lead_services
SET status = 'In_Progress'
WHERE status = 'Completed';

ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_status_check;

ALTER TABLE RAC_lead_services
ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
        'New',
        'Pending',
        'In_Progress',
        'Attempted_Contact',
        'Appointment_Scheduled',
        'Needs_Rescheduling',
        'Disqualified'
    )
);
