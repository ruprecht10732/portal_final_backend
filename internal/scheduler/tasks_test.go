package scheduler

import "testing"

const testGatekeeperTaskTenantID = "tenant-1"
const testGatekeeperTaskLeadID = "lead-1"
const testGatekeeperTaskServiceID = "service-1"

func TestNewGatekeeperRunTaskOmitsFingerprintFromTaskPayload(t *testing.T) {
	task, err := NewGatekeeperRunTask(GatekeeperRunPayload{
		TenantID:      testGatekeeperTaskTenantID,
		LeadID:        testGatekeeperTaskLeadID,
		LeadServiceID: testGatekeeperTaskServiceID,
		Fingerprint:   "fingerprint-123",
	})
	if err != nil {
		t.Fatalf("expected gatekeeper task to marshal, got %v", err)
	}

	payload, err := ParseGatekeeperRunPayload(task)
	if err != nil {
		t.Fatalf("expected gatekeeper task payload to parse, got %v", err)
	}

	if payload.TenantID != testGatekeeperTaskTenantID || payload.LeadID != testGatekeeperTaskLeadID || payload.LeadServiceID != testGatekeeperTaskServiceID {
		t.Fatalf("unexpected gatekeeper task payload: %#v", payload)
	}
	if payload.Fingerprint != "" {
		t.Fatalf("expected queued gatekeeper task payload to omit fingerprint, got %q", payload.Fingerprint)
	}
}

func TestNewGatekeeperRunTaskNormalizesPayloadAcrossFingerprints(t *testing.T) {
	taskOne, err := NewGatekeeperRunTask(GatekeeperRunPayload{
		TenantID:      testGatekeeperTaskTenantID,
		LeadID:        testGatekeeperTaskLeadID,
		LeadServiceID: testGatekeeperTaskServiceID,
		Fingerprint:   "fingerprint-a",
	})
	if err != nil {
		t.Fatalf("expected first gatekeeper task to marshal, got %v", err)
	}

	taskTwo, err := NewGatekeeperRunTask(GatekeeperRunPayload{
		TenantID:      testGatekeeperTaskTenantID,
		LeadID:        testGatekeeperTaskLeadID,
		LeadServiceID: testGatekeeperTaskServiceID,
		Fingerprint:   "fingerprint-b",
	})
	if err != nil {
		t.Fatalf("expected second gatekeeper task to marshal, got %v", err)
	}

	if string(taskOne.Payload()) != string(taskTwo.Payload()) {
		t.Fatalf("expected gatekeeper task payloads to match across fingerprints, got %s vs %s", string(taskOne.Payload()), string(taskTwo.Payload()))
	}
}
