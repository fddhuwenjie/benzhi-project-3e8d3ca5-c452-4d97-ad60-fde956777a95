package application

import (
	"errors"
	"testing"
	"time"

	"siteclosure/internal/domain"
)

func createAndFreeze(t *testing.T, s *Service, id string) *domain.ClosureCase {
	t.Helper()
	_, err := s.CreateCase(CreateCaseRequest{RequestID: id + "-create", CaseID: id, SiteCode: "SITE-X", Coordinates: "N1/E1", CompletionDigest: "sha256:record", Surface: "稳定", Actor: "记录员"})
	if err != nil {
		t.Fatal(err)
	}
	precheck, err := s.PrecheckBaseline(id, "记录员", []string{"记录员", "施工员"})
	if err != nil || !precheck.CanFreeze {
		t.Fatalf("precheck=%#v err=%v", precheck, err)
	}
	request := BaselineRequest{RequestID: id + "-freeze", CaseID: id, Actor: "记录员", People: []string{"记录员", "施工员"}, ExpectedRevision: 1, ConfirmedDigest: precheck.SummaryDigest}
	first, err := s.FreezeBaseline(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.FreezeBaseline(request)
	if err != nil || first == nil || second == nil {
		t.Fatalf("冻结重放失败: %v", err)
	}
	c, _ := s.GetCase(id)
	return c
}

func prepareAndApprove(t *testing.T, s *Service, id string, layers []domain.LayerSpec) *domain.ClosureCase {
	t.Helper()
	c := createAndFreeze(t, s, id)
	if _, err := s.PreparePlan(PlanRequest{RequestID: id + "-plan", CaseID: id, PreparedBy: "编制员", Layers: layers, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(id)
	risks := domain.RiskCodes(c.Plan.Review)
	if _, err := s.ApprovePlan(ApproveRequest{RequestID: id + "-approve", CaseID: id, Actor: "批准员", ExpectedRevision: c.Revision, PlanDigest: c.Plan.PlanDigest, ConfirmedRiskCodes: risks}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(id)
	return c
}

func standardLayer(index int) domain.LayerSpec {
	return domain.LayerSpec{Index: index, MaterialCode: "soil", TargetThicknessMM: 200, ThicknessToleranceMM: 10, MoistureMinPercent: 12, MoistureMaxPercent: 18, CompactionMinPercent: 90, EvidenceRequired: true}
}

func TestBaselineReceiptPrecheckAndIdempotency(t *testing.T) {
	s := newService(t)
	_, err := s.CreateCase(CreateCaseRequest{RequestID: "b-create", CaseID: "baseline", SiteCode: "SITE", Coordinates: "N/E", CompletionDigest: "digest", Surface: "稳定", Actor: "记录员"})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := s.PrecheckBaseline("baseline", "记录员", []string{"记录员", "记录员"})
	if err != nil || bad.CanFreeze || len(bad.Issues) != 1 {
		t.Fatalf("bad=%#v err=%v", bad, err)
	}
	good, err := s.PrecheckBaseline("baseline", "记录员", []string{"记录员"})
	if err != nil || !good.CanFreeze {
		t.Fatal(err)
	}
	r := BaselineRequest{RequestID: "b-freeze", CaseID: "baseline", Actor: "记录员", People: []string{"记录员"}, ExpectedRevision: 1, ConfirmedDigest: good.SummaryDigest}
	if _, err = s.FreezeBaseline(r); err != nil {
		t.Fatal(err)
	}
	r.People = []string{"其他人"}
	if _, err = s.FreezeBaseline(r); err == nil {
		t.Fatal("同 request_id 异载荷未冲突")
	}
	c, _ := s.GetCase("baseline")
	if c.Revision != 2 || c.BaselineReceipt == nil || !c.BaselineIntegrity().Valid {
		t.Fatalf("case=%#v", c)
	}
}

func TestPlanRiskDigestAndLayerDraft(t *testing.T) {
	s := newService(t)
	layer := standardLayer(1)
	layer.ThicknessToleranceMM = 40
	c := createAndFreeze(t, s, "draft-flow")
	if _, err := s.PreparePlan(PlanRequest{RequestID: "d-plan", CaseID: c.CaseID, PreparedBy: "编制员", Layers: []domain.LayerSpec{layer}, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if len(c.Plan.Review.Risks) == 0 {
		t.Fatal("未生成容差风险")
	}
	if _, err := s.ApprovePlan(ApproveRequest{RequestID: "d-bad-approve", CaseID: c.CaseID, Actor: "批准员", ExpectedRevision: c.Revision, PlanDigest: c.Plan.PlanDigest}); err == nil {
		t.Fatal("未确认风险应阻断")
	}
	if _, err := s.ApprovePlan(ApproveRequest{RequestID: "d-approve", CaseID: c.CaseID, Actor: "批准员", ExpectedRevision: c.Revision, PlanDigest: c.Plan.PlanDigest, ConfirmedRiskCodes: domain.RiskCodes(c.Plan.Review)}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	revision := c.Revision
	thickness := 200
	draft, err := s.SaveLayerDraft(DraftRequest{CaseID: c.CaseID, LayerIndex: 1, MaterialCode: "soil", ThicknessMM: &thickness})
	if err != nil {
		t.Fatal(err)
	}
	if draft.DraftVersion != 1 || len(draft.MissingFields) != 4 {
		t.Fatalf("draft=%#v", draft)
	}
	c, _ = s.GetCase(c.CaseID)
	if c.Revision != revision {
		t.Fatal("草稿修改了案件 revision")
	}
	moisture, compaction := float64(12), float64(90)
	draft, err = s.SaveLayerDraft(DraftRequest{CaseID: c.CaseID, LayerIndex: 1, MaterialCode: "soil", ThicknessMM: &thickness, Moisture: &moisture, Compaction: &compaction, PerformedBy: "施工员", EvidenceDigest: "sha256:e", ExpectedDraftVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitLayerDraft(c.CaseID, "d-submit-old", "exec", revision, 1); err == nil {
		t.Fatal("旧草稿版本未被拒绝")
	}
	if _, err = s.SubmitLayerDraft(c.CaseID, "d-submit", "exec", revision, draft.DraftVersion); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.store.GetDraft(c.CaseID); ok {
		t.Fatal("正式提交后草稿未清除")
	}
	c, _ = s.GetCase(c.CaseID)
	if len(c.Executions) != 1 || len(c.Executions[0].Evaluation.Rules) != 5 {
		t.Fatalf("case=%#v", c)
	}
}

func TestStagedRemediationKeepsAttempts(t *testing.T) {
	s := newService(t)
	c := prepareAndApprove(t, s, "remediation", []domain.LayerSpec{standardLayer(1)})
	_, err := s.SubmitLayer(LayerRequest{RequestID: "r-layer", CaseID: c.CaseID, ExecutionID: "exec", LayerIndex: 1, MaterialCode: "soil", ThicknessMM: 200, Moisture: 20, Compaction: 80, PerformedBy: "施工员", EvidenceDigest: "e", ExpectedRevision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	d := c.OpenDefects()[0]
	planned := time.Now().UTC().Add(time.Hour)
	if _, err = s.PlanRemediation(RemediationPlanRequest{RequestID: "r-plan", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "负责人", CauseCategory: "施工参数", Cause: "参数偏差", CorrectiveAction: "调湿压实", Responsible: "施工员", PlannedCompletionAt: planned, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if _, err = s.CompleteRemediation(RemediationCompleteRequest{RequestID: "r-complete", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "施工员", Description: "已完成", Evidence: "fix-evidence", ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if _, err = s.RetestStaged(RetestRequest{RequestID: "r-missing", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "复验员", Evidence: "retest", Values: map[string]float64{"moisture_percent": 20}, ExpectedRevision: c.Revision}); err == nil {
		t.Fatal("缺少压实复验值未阻断")
	}
	if _, err = s.RetestStaged(RetestRequest{RequestID: "r-fail", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "复验员", Evidence: "retest-1", Values: map[string]float64{"moisture_percent": 20, "compaction_percent": 92}, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if len(c.Defects[0].RetestAttempts) != 1 || c.Defects[0].Status != domain.DefectOpen {
		t.Fatalf("defect=%#v", c.Defects[0])
	}
	if _, err = s.RetestStaged(RetestRequest{RequestID: "r-pass", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "复验员", Evidence: "retest-2", Values: map[string]float64{"moisture_percent": 18, "compaction_percent": 90}, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if c.Status != domain.StatusAwaitingReview || len(c.Defects[0].RetestAttempts) != 2 {
		t.Fatalf("case=%#v", c)
	}
}

func TestReviewReturnListAndVerificationReport(t *testing.T) {
	s := newService(t)
	layers := []domain.LayerSpec{standardLayer(1), standardLayer(2)}
	c := prepareAndApprove(t, s, "review", layers)
	for i := 1; i <= 2; i++ {
		if _, err := s.SubmitLayer(LayerRequest{RequestID: "review-layer-" + string(rune('0'+i)), CaseID: c.CaseID, ExecutionID: "exec-" + string(rune('0'+i)), LayerIndex: i, MaterialCode: "soil", ThicknessMM: 200, Moisture: 15, Compaction: 92, PerformedBy: "施工员", EvidenceDigest: "e", ExpectedRevision: c.Revision}); err != nil {
			t.Fatal(err)
		}
		c, _ = s.GetCase(c.CaseID)
	}
	issues := []domain.ReviewIssue{{LayerIndex: 1, Description: "边缘松散", RequiredAction: "补充夯实", RequiredEvidenceType: "照片"}, {LayerIndex: 2, Description: "标识缺失", RequiredAction: "补充标识", RequiredEvidenceType: "记录"}}
	if _, err := s.Review(ReviewRequest{RequestID: "review-return", CaseID: c.CaseID, Actor: "验收员甲", Decision: "return", Issues: issues, ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCase(c.CaseID)
	if len(c.OpenDefects()) != 2 {
		t.Fatal("验收问题未分别生成缺陷")
	}
	for i, d := range c.OpenDefects() {
		prefix := "review-fix-" + string(rune('0'+i))
		if _, err := s.PlanRemediation(RemediationPlanRequest{RequestID: prefix + "p", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "负责人", CauseCategory: "验收退回", Cause: "需补充", CorrectiveAction: "按要求完成", Responsible: "施工员", PlannedCompletionAt: time.Now().Add(time.Hour), ExpectedRevision: c.Revision}); err != nil {
			t.Fatal(err)
		}
		c, _ = s.GetCase(c.CaseID)
		if _, err := s.CompleteRemediation(RemediationCompleteRequest{RequestID: prefix + "c", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "施工员", Description: "完成", Evidence: "fix", ExpectedRevision: c.Revision}); err != nil {
			t.Fatal(err)
		}
		c, _ = s.GetCase(c.CaseID)
		if _, err := s.RetestStaged(RetestRequest{RequestID: prefix + "r", CaseID: c.CaseID, DefectID: d.DefectID, Actor: "复验员", Evidence: "retest", Values: map[string]float64{"review_confirmed": 1}, ExpectedRevision: c.Revision}); err != nil {
			t.Fatal(err)
		}
		c, _ = s.GetCase(c.CaseID)
	}
	if c.Status != domain.StatusAwaitingReview {
		t.Fatalf("status=%s", c.Status)
	}
	if _, err := s.Review(ReviewRequest{RequestID: "review-pass", CaseID: c.CaseID, Actor: "验收员乙", Decision: "pass", ExpectedRevision: c.Revision}); err != nil {
		t.Fatal(err)
	}
	report, err := s.VerificationReport(c.CaseID)
	if err != nil || !report.Valid {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	filter := true
	list, err := s.ListCases(CaseFilter{SiteCode: "SITE-X", HasOpenDefect: &filter, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 || list.Stats.Total != 0 {
		t.Fatalf("list=%#v", list)
	}
	_, err = s.ListCases(CaseFilter{Status: "unknown", Page: 1, PageSize: 20})
	var de domain.DomainError
	if !errors.As(err, &de) || de.Code != "INVALID_STATUS" {
		t.Fatalf("err=%v", err)
	}
}
