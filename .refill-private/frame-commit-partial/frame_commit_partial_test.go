package framecommitpartial

import (
	"os"
	"path/filepath"
	"testing"

	"siteclosure/internal/application"
	"siteclosure/internal/storage"
)

func TestFailedFrameWriteDoesNotPersistCommittedState(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	caseID := "blocked-frame"
	blockedTarget := filepath.Join(dir, "events", caseID+".frames")
	if err := os.MkdirAll(blockedTarget, 0755); err != nil {
		t.Fatal(err)
	}

	service := application.New(store)
	_, err = service.CreateCase(application.CreateCaseRequest{
		RequestID:        "blocked-frame-create",
		CaseID:           caseID,
		SiteCode:         "SITE",
		Coordinates:      "N1/E1",
		CompletionDigest: "sha256:record",
		Surface:          "stable",
		Actor:            "recorder",
	})
	if err == nil {
		t.Fatal("frame write obstruction did not fail the request")
	}

	if _, err := storage.New(dir); err != nil {
		t.Fatalf("failed frame write left an unrecoverable committed snapshot: %v", err)
	}
}
