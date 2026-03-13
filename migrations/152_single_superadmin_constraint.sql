-- +goose Up

-- +goose StatementBegin
DO $$
DECLARE
    superadmin_role_id UUID;
BEGIN
    SELECT id INTO superadmin_role_id
    FROM RAC_roles
    WHERE name = 'superadmin'
    LIMIT 1;

    IF superadmin_role_id IS NULL THEN
        RAISE EXCEPTION 'superadmin role must exist before creating single-superadmin constraint';
    END IF;

    EXECUTE format(
        'CREATE UNIQUE INDEX ux_single_superadmin_user ON RAC_user_roles (role_id) WHERE role_id = %L::uuid',
        superadmin_role_id
    );
END $$;
-- +goose StatementEnd

-- +goose Down

DROP INDEX IF EXISTS ux_single_superadmin_user;
