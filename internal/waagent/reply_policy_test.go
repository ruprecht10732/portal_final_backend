package waagent

import "testing"

func TestValidateReplyPolicyRejectsLookupProcessNarration(t *testing.T) {
	t.Parallel()

	issue := validateReplyPolicy(agentRunModeLookup, "Ik ga dat opzoeken. Laat me dat zoeken.")
	if issue != "process_narration" {
		t.Fatalf("expected process narration issue, got %q", issue)
	}
}

func TestValidateReplyPolicyRejectsLongLookupReply(t *testing.T) {
	t.Parallel()

	issue := validateReplyPolicy(agentRunModeLookup, "Een. Twee. Drie. Vier. Vijf. Zes.")
	if issue != "lookup_reply_too_long" {
		t.Fatalf("expected lookup length issue, got %q", issue)
	}
}

func TestValidateReplyPolicyAllowsDefaultModeNarration(t *testing.T) {
	t.Parallel()

	issue := validateReplyPolicy(agentRunModeDefault, "Ik ga dat voor je regelen.")
	if issue != "" {
		t.Fatalf("expected no issue for default mode, got %q", issue)
	}
}
