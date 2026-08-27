package domain

import (
	"reflect"
	"testing"
	"time"
)

func testCase(t *testing.T) *ClosureCase {
	t.Helper()
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	c, err := NewCase("case-1", "SITE", "N1/E2", "record", "stable", "creator", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.FreezeBaseline("creator", []string{"creator"}, now); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPlan(BackfillPlan{PlanID: "plan-1", CaseID: c.CaseID, PreparedBy: "planner", Layers: []LayerSpec{{Index: 1, MaterialCode: "soil", TargetThicknessMM: 200, ThicknessToleranceMM: 10, MoistureMinPercent: 12, MoistureMaxPercent: 18, CompactionMinPercent: 90, EvidenceRequired: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.ApprovePlan("approver", now); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEvaluateLayerStableRules(t *testing.T) {
	spec := LayerSpec{Index: 1, MaterialCode: "soil", TargetThicknessMM: 200, ThicknessToleranceMM: 10, MoistureMinPercent: 12, MoistureMaxPercent: 18, CompactionMinPercent: 90, EvidenceRequired: true}
	got := EvaluateLayer(spec, "sand", 225, 9, 80, "")
	want := []string{"MATERIAL_MISMATCH", "THICKNESS_OUT_OF_RANGE", "MOISTURE_OUT_OF_RANGE", "COMPACTION_BELOW_THRESHOLD", "EVIDENCE_REQUIRED"}
	if got.Verdict != VerdictFail || !reflect.DeepEqual(got.Failed, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestDefectBlocksAndRetestRestoresFlow(t *testing.T) {
	c := testCase(t)
	now := time.Now().UTC()
	failed, err := c.SubmitLayer(LayerExecution{ExecutionID: "exec", CaseID: c.CaseID, LayerIndex: 1, MaterialCode: "sand", ActualThicknessMM: 230, MoisturePercent: 9, CompactionPercent: 80, PerformedBy: "worker", EvidenceDigest: "bad"}, now)
	if err != nil || len(failed) != 4 {
		t.Fatalf("failed=%v err=%v", failed, err)
	}
	if err := c.AddDefect(DefectRecord{DefectID: "defect", CaseID: c.CaseID, LayerIndex: 1, FailedRuleCodes: failed}); err != nil {
		t.Fatal(err)
	}
	_, err = c.SubmitLayer(LayerExecution{LayerIndex: 1}, now)
	if err == nil {
		t.Fatal("开放缺陷未阻断施工")
	}
	values := map[string]float64{"thickness_mm": 200, "moisture_percent": 15, "compaction_percent": 92}
	if err := c.RetestDefect("defect", "inspector", "evidence", values, now); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusAwaitingReview || c.Executions[0].Verdict != VerdictPass {
		t.Fatalf("status=%s verdict=%s", c.Status, c.Executions[0].Verdict)
	}
}

func TestDutySeparationAndReviewReturn(t *testing.T) {
	c := testCase(t)
	now := time.Now().UTC()
	_, err := c.SubmitLayer(LayerExecution{ExecutionID: "exec", CaseID: c.CaseID, LayerIndex: 1, MaterialCode: "soil", ActualThicknessMM: 200, MoisturePercent: 15, CompactionPercent: 91, PerformedBy: "worker", EvidenceDigest: "ok"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Review("worker", "pass", now); err == nil {
		t.Fatal("施工人员不应可验收")
	}
	if err := c.Review("reviewer", "return", now); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusRemediation || len(c.OpenDefects()) != 1 {
		t.Fatalf("退回未生成整改缺陷: %#v", c)
	}
}
