package repository

import (
	"strings"
	"testing"
)

func TestListUsersByOrganizationQueryIsTenantScoped(t *testing.T) {
	query := strings.ToLower(listUsersByOrganizationQuery)

	requiredFragments := []string{
		"from rac_organization_members om",
		"join rac_users u on u.id = om.user_id",
		"where om.organization_id = $1",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected tenant-scoped query fragment %q to be present", fragment)
		}
	}
}

func TestListUsersQueryHasNoOrganizationJoin(t *testing.T) {
	query := strings.ToLower(listUsersQuery)

	if strings.Contains(query, "rac_organization_members") {
		t.Fatal("global list users query should not include organization membership join")
	}
}
