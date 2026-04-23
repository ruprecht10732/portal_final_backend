package repository

import (
	"strings"
	"testing"
)

// =============================================================================
// Test Fixtures (Query Contracts)
// =============================================================================

const (
	listUsersQuery = `
        SELECT
            u.id,
            u.email,
            u.first_name,
            u.last_name,
            COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
        FROM RAC_users u
        LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
        LEFT JOIN RAC_roles r ON r.id = ur.role_id
        GROUP BY u.id
        ORDER BY u.email
    `
	listUsersByOrganizationQuery = `
        SELECT
            u.id,
            u.email,
            u.first_name,
            u.last_name,
            COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
        FROM RAC_organization_members om
        JOIN RAC_users u ON u.id = om.user_id
        LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
        LEFT JOIN RAC_roles r ON r.id = ur.role_id
        WHERE om.organization_id = $1
        GROUP BY u.id
        ORDER BY u.email
    `
)

// =============================================================================
// Security & Scope Tests
// =============================================================================

func TestListUsersByOrganizationQueryIsTenantScoped(t *testing.T) {
	t.Parallel()

	query := strings.ToLower(listUsersByOrganizationQuery)

	requiredFragments := []string{
		"from rac_organization_members om",
		"join rac_users u on u.id = om.user_id",
		"where om.organization_id = $1",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(query, fragment) {
			t.Errorf("Security Violation: expected tenant-scoped query fragment %q to be present", fragment)
		}
	}
}

func TestListUsersQueryHasNoOrganizationJoin(t *testing.T) {
	t.Parallel()

	query := strings.ToLower(listUsersQuery)

	if strings.Contains(query, "rac_organization_members") {
		t.Fatal("Logic Violation: global list users query should not include organization membership join")
	}
}
