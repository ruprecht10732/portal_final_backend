package leadsdb

import (
	"strings"
	"testing"
)

func TestListActionItemsCoalescesUrgencyLevel(t *testing.T) {
	if !strings.Contains(listActionItems, "COALESCE(ai.urgency_level, '') AS urgency_level") {
		t.Fatalf("expected listActionItems to coalesce nullable urgency_level")
	}
}