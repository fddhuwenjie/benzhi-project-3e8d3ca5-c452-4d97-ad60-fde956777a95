package reviewdossierpartial

import (
	"os"
	"path/filepath"
	"testing"

	"siteclosure/internal/application"
	"siteclosure/internal/domain"
	"siteclosure/internal/storage"
)

func TestDossierFailureDoesNotSealCase(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(store)
	caseID := "dossier-failure"
	caseState := prepareAwaitingReview(t, service, caseID)

	blockedTarget := filepath.Join(dir, "dossiers", caseID+".json")
	if err := os.MkdirAll(blockedTarget, 0755); err != nil {
		t.Fatal(err)
	}
	_, err = service.Review(application.ReviewRequest{
		RequestID:        "dossier-failure-review",
		CaseID:           caseID,
		Actor:            "independent-reviewer",
		Decision:         "pass",
		ExpectedRevision: caseState.Revision,
	})
	if err == nil {
		t.Fatal("dossier obstruction did not fail review")
	}

	caseState, err = service.GetCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if caseState.Status != domain.StatusAwaitingReview || caseState.SealedAt != nil {
		t.Fatalf("failed dossier persistence still sealed case: status=%s", caseState.Status)
	}
}

func prepareAwaitingReview(t *testing.T, service *application.Service, caseID string) *domain.ClosureCase {
	t.Helper()
	if _, err := service.CreateCase(application.CreateCaseRequest{
		RequestID:        caseID + "-create",
		CaseID:           caseID,
		SiteCode:         "SITE",
		Coordinates:      "N1/E1",
		CompletionDigest: "sha256:record",
		Surface:          "stable",
		Actor:            "recorder",
	}); err != nil {
		t.Fatal(err)
	}
	precheck, err := service.PrecheckBaseline(caseID, "recorder", []string{"recorder"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FreezeBaseline(application.BaselineRequest{RequestID: caseID + "-freeze", CaseID: caseID, Actor: "recorder", People: []string{"recorder"}, ExpectedRevision: 1, ConfirmedDigest: precheck.SummaryDigest}); err != nil {
		t.Fatal(err)
	}
	caseState, err := service.GetCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	layers := []domain.LayerSpec{{Index: 1, MaterialCode: "soil", TargetThicknessMM: 200, ThicknessToleranceMM: 10, MoistureMinPercent: 12, MoistureMaxPercent: 18, CompactionMinPercent: 90, EvidenceRequired: true}}
	if _, err := service.PreparePlan(application.PlanRequest{RequestID: caseID + "-plan", CaseID: caseID, PreparedBy: "planner", Layers: layers, ExpectedRevision: caseState.Revision}); err != nil {
		t.Fatal(err)
	}
	caseState, err = service.GetCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApprovePlan(application.ApproveRequest{RequestID: caseID + "-approve", CaseID: caseID, Actor: "approver", ExpectedRevision: caseState.Revision, PlanDigest: caseState.Plan.PlanDigest, ConfirmedRiskCodes: domain.RiskCodes(caseState.Plan.Review)}); err != nil {
		t.Fatal(err)
	}
	caseState, err = service.GetCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitLayer(application.LayerRequest{RequestID: caseID + "-layer", CaseID: caseID, ExecutionID: "execution-1", LayerIndex: 1, MaterialCode: "soil", ThicknessMM: 200, Moisture: 15, Compaction: 92, PerformedBy: "worker", EvidenceDigest: "sha256:evidence", ExpectedRevision: caseState.Revision}); err != nil {
		t.Fatal(err)
	}
	caseState, err = service.GetCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	return caseState
}
