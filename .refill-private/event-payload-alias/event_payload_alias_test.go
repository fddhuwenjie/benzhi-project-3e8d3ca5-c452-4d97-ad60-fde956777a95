package eventpayloadalias

import (
	"testing"

	"siteclosure/internal/application"
	"siteclosure/internal/storage"
)

func TestEventsResultCannotCorruptRecoveredState(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(store)
	caseID := "event-alias-case"
	if _, err := service.CreateCase(application.CreateCaseRequest{
		RequestID:        "event-alias-create",
		CaseID:           caseID,
		SiteCode:         "SITE",
		Coordinates:      "N1/E1",
		CompletionDigest: "sha256:record",
		Surface:          "stable",
		Actor:            "recorder",
	}); err != nil {
		t.Fatal(err)
	}

	events := service.Events(caseID)
	if len(events) != 1 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	events[0].Payload["site_code"] = "tampered-by-reader"

	precheck, err := service.PrecheckBaseline(caseID, "recorder", []string{"recorder"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FreezeBaseline(application.BaselineRequest{
		RequestID:        "event-alias-freeze",
		CaseID:           caseID,
		Actor:            "recorder",
		People:           []string{"recorder"},
		ExpectedRevision: 1,
		ConfirmedDigest:  precheck.SummaryDigest,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.New(dir); err != nil {
		t.Fatalf("read-only Events result corrupted recovered state: %v", err)
	}
}
