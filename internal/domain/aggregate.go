package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func NewCase(id, site, coordinates, digest, surface, actor string, now time.Time) (*ClosureCase, error) {
	if err := ValidateNewCaseFields(id, site, coordinates, digest, surface, actor); err != nil {
		return nil, err
	}
	return &ClosureCase{CaseID: id, SiteCode: site, TrenchCoordinates: coordinates, CompletionRecordDigest: digest, ExposedSurfaceCondition: surface, Status: StatusDraft, Revision: 1, CreatedBy: actor, CreatedAt: now}, nil
}

func (c *ClosureCase) FreezeBaseline(actor string, people []string, now time.Time) error {
	_, digest := BaselinePrecheck(c, actor, people)
	return c.FreezeBaselineConfirmed(actor, people, digest, now)
}

func (c *ClosureCase) FreezeBaselineConfirmed(actor string, people []string, confirmedDigest string, now time.Time) error {
	if c.Status != StatusDraft {
		return DomainError{"STATE", "只有草拟案件可冻结基线"}
	}
	if err := ValidateActor(actor); err != nil {
		return err
	}
	if err := ValidateResponsiblePeople(people); err != nil {
		return err
	}
	issues, digest := BaselinePrecheck(c, actor, people)
	if len(issues) > 0 {
		return DomainError{"BASELINE_PRECHECK", "冻结预检存在阻断项"}
	}
	if confirmedDigest != digest {
		return DomainError{"BASELINE_DIGEST", "确认的基线摘要与当前内容不匹配"}
	}
	baseline := Baseline{SiteCode: strings.TrimSpace(c.SiteCode), TrenchCoordinates: strings.TrimSpace(c.TrenchCoordinates), CompletionRecordDigest: strings.TrimSpace(c.CompletionRecordDigest), ExposedSurfaceCondition: strings.TrimSpace(c.ExposedSurfaceCondition), ResponsiblePeople: normalizedPeople(people), FrozenAt: now.UTC(), FrozenBy: actor}
	receipt := BaselineReceipt{CaseID: c.CaseID, NormalizedContent: baseline, FrozenBy: actor, FrozenAt: now.UTC(), SummaryDigest: digest}
	baseline.ReceiptDigest = CanonicalDigest(receipt)
	c.Baseline = &baseline
	c.BaselineReceipt = &receipt
	c.Status = StatusFrozen
	c.Revision++
	return nil
}

func (c *ClosureCase) SetPlan(plan BackfillPlan) error {
	if c.Status != StatusFrozen {
		return DomainError{"STATE", "只有冻结基线后才能编制方案"}
	}
	if plan.PreparedBy == "" || plan.PreparedBy == plan.ApprovedBy {
		return DomainError{"DUTY_SEPARATION", "编制人与批准人必须分离"}
	}
	if err := ValidateActor(plan.PreparedBy); err != nil {
		return err
	}
	if !c.BaselineIntegrity().Valid {
		return DomainError{"BASELINE_INTEGRITY", "冻结基线完整性异常，禁止编制方案"}
	}
	if err := ValidatePlan(plan.Layers); err != nil {
		return err
	}
	if plan.CaseID != c.CaseID {
		return DomainError{"CASE_ID", "方案案件不匹配"}
	}
	plan.Review = BuildPlanReview(plan.Layers)
	plan.PlanDigest = CanonicalDigest(struct {
		CaseID     string      `json:"case_id"`
		PreparedBy string      `json:"prepared_by"`
		Layers     []LayerSpec `json:"layers"`
		Review     PlanReview  `json:"review"`
	}{plan.CaseID, plan.PreparedBy, plan.Layers, plan.Review})
	c.Plan = &plan
	c.Revision++
	return nil
}

func (c *ClosureCase) ApprovePlan(actor string, now time.Time) error {
	if c.Plan == nil {
		return DomainError{"STATE", "当前不可批准方案"}
	}
	return c.ApprovePlanChecked(actor, c.Plan.PlanDigest, RiskCodes(c.Plan.Review), now)
}

func (c *ClosureCase) ApprovePlanChecked(actor, planDigest string, confirmedRisks []string, now time.Time) error {
	if c.Status != StatusFrozen || c.Plan == nil {
		return DomainError{"STATE", "当前不可批准方案"}
	}
	if err := ValidateActor(actor); err != nil {
		return err
	}
	if actor == c.Plan.PreparedBy {
		return DomainError{"DUTY_SEPARATION", "批准人必须与编制人不同"}
	}
	if !c.BaselineIntegrity().Valid {
		return DomainError{"BASELINE_INTEGRITY", "冻结基线完整性异常，禁止批准方案"}
	}
	if planDigest != c.Plan.PlanDigest {
		return DomainError{"PLAN_DIGEST_CONFLICT", "方案摘要已变化，请重新复核"}
	}
	confirmed := map[string]bool{}
	for _, code := range confirmedRisks {
		confirmed[code] = true
	}
	for _, code := range RiskCodes(c.Plan.Review) {
		if !confirmed[code] {
			return DomainError{"UNCONFIRMED_RISK", "仍有未确认风险: " + code}
		}
	}
	c.Plan.ApprovedBy = actor
	c.Plan.ApprovedAt = &now
	c.Plan.Revision = c.Revision + 1
	c.Plan.ConfirmedRiskCodes = append([]string(nil), confirmedRisks...)
	c.Status = StatusPlanReady
	c.Revision++
	return nil
}

func (c *ClosureCase) NextLayer() int { return len(c.Executions) + 1 }
func (c *ClosureCase) OpenDefects() []DefectRecord {
	out := []DefectRecord{}
	for _, d := range c.Defects {
		if d.Status == DefectOpen {
			out = append(out, d)
		}
	}
	return out
}
func (c *ClosureCase) WorkerIDs() map[string]bool {
	m := map[string]bool{}
	for _, e := range c.Executions {
		m[e.PerformedBy] = true
	}
	return m
}

func (c *ClosureCase) SubmitLayer(ex LayerExecution, now time.Time) ([]string, error) {
	if err := ValidateExecutionInput(ex); err != nil {
		return nil, err
	}
	if c.Status != StatusPlanReady && c.Status != StatusInProgress {
		return nil, DomainError{"STATE", "当前不允许施工"}
	}
	if len(c.OpenDefects()) > 0 {
		return nil, DomainError{"OPEN_DEFECT", "存在开放缺陷，必须先整改复验"}
	}
	if c.Plan == nil || ex.LayerIndex != c.NextLayer() || ex.LayerIndex > len(c.Plan.Layers) {
		return nil, DomainError{"LAYER_ORDER", "必须按批准顺序提交施工层"}
	}
	spec := c.Plan.Layers[ex.LayerIndex-1]
	result := EvaluateLayer(spec, ex.MaterialCode, ex.ActualThicknessMM, ex.MoisturePercent, ex.CompactionPercent, ex.EvidenceDigest)
	ex.SubmittedAt = now
	ex.Verdict = result.Verdict
	ex.FailedRuleCodes = result.Failed
	ex.Evaluation = EvaluationSnapshot{RuleVersion: RuleVersion, PlanDigest: c.Plan.PlanDigest, Rules: result.Details, ResultDigest: result.Digest}
	c.Executions = append(c.Executions, ex)
	c.Revision++
	if result.Verdict == VerdictFail {
		c.Status = StatusRemediation
	} else {
		c.Status = StatusInProgress
		if len(c.Executions) == len(c.Plan.Layers) {
			c.Status = StatusAwaitingReview
		}
	}
	return result.Failed, nil
}

func (c *ClosureCase) AddDefect(d DefectRecord) error {
	if c.Status != StatusRemediation {
		return DomainError{"STATE", "当前没有可登记缺陷的施工状态"}
	}
	if len(d.FailedRuleCodes) == 0 {
		return DomainError{"DEFECT_RULES", "缺陷必须关联失败规则"}
	}
	d.Status = DefectOpen
	c.Defects = append(c.Defects, d)
	c.Revision++
	return nil
}

func (c *ClosureCase) RetestDefect(id, actor, evidence string, values map[string]float64, now time.Time) error {
	if err := ValidateRetestValues(values); err != nil {
		return err
	}
	d, err := c.findOpenDefect(id)
	if err != nil {
		return err
	}
	if len(d.Plans) == 0 {
		_ = c.RecordRemediationPlan(id, "legacy", "旧版一次性整改", "完成定向复验", actor, now, actor, now)
	}
	if d.Completion == nil {
		_ = c.CompleteRemediation(id, "旧版一次性整改完成", evidence, actor, now)
	}
	values["material_confirmed"], values["evidence_confirmed"], values["review_confirmed"] = 1, 1, 1
	passed, remaining, err := c.RetestCompletedDefect(id, actor, evidence, values, now)
	if err != nil {
		return err
	}
	if !passed {
		return DomainError{"RETEST_FAILED", "复验仍未通过: " + strings.Join(remaining, ",")}
	}
	for i := range c.Executions {
		if c.Executions[i].LayerIndex == d.LayerIndex {
			c.Executions[i].Verdict = VerdictPass
			c.Executions[i].FailedRuleCodes = nil
		}
	}
	if c.ReadyForReview() {
		c.Status = StatusAwaitingReview
	}
	return nil
}

func (c *ClosureCase) Review(actor, decision string, now time.Time) error {
	issues := []ReviewIssue(nil)
	if decision == "return" {
		issues = []ReviewIssue{{IssueID: fmt.Sprintf("issue-%d", now.UnixNano()), LayerIndex: len(c.Plan.Layers), Description: "独立验收退回", RequiredAction: "按验收要求整改", RequiredEvidenceType: "整改证据"}}
	}
	return c.ReviewWithIssues(actor, decision, issues, now)
}

func (c *ClosureCase) ReviewWithIssues(actor, decision string, issues []ReviewIssue, now time.Time) error {
	if c.Status != StatusAwaitingReview {
		return DomainError{"STATE", "尚未达到待验收状态"}
	}
	if actor == "" || actor == c.Plan.PreparedBy || actor == c.Plan.ApprovedBy || c.WorkerIDs()[actor] {
		return DomainError{"DUTY_SEPARATION", "验收员必须与编制、批准和施工人员分离"}
	}
	if decision != "pass" && decision != "return" {
		return DomainError{"DECISION", "验收结论必须为pass或return"}
	}
	if decision == "return" && len(issues) == 0 {
		return DomainError{"REVIEW_ISSUES", "退回结论至少需要一个问题项"}
	}
	if decision == "pass" && len(issues) > 0 {
		return DomainError{"REVIEW_ISSUES", "通过结论不得包含退回问题项"}
	}
	for i := range issues {
		if issues[i].LayerIndex < 1 || c.Plan == nil || issues[i].LayerIndex > len(c.Plan.Layers) {
			return DomainError{"REVIEW_ISSUES", "验收问题关联层无效"}
		}
		if strings.TrimSpace(issues[i].Description) == "" || strings.TrimSpace(issues[i].RequiredAction) == "" || strings.TrimSpace(issues[i].RequiredEvidenceType) == "" {
			return DomainError{"REVIEW_ISSUES", "验收问题说明、纠正措施和证据类型不能为空"}
		}
		if issues[i].IssueID == "" {
			issues[i].IssueID = fmt.Sprintf("review-%d-%d", now.UnixNano(), i+1)
		}
	}
	c.ReviewerID = actor
	c.ReviewDecision = decision
	c.ReviewHistory = append(c.ReviewHistory, ReviewRound{Round: len(c.ReviewHistory) + 1, ReviewerID: actor, Decision: decision, Issues: append([]ReviewIssue(nil), issues...), ReviewedAt: now.UTC()})
	c.Revision++
	if decision == "return" {
		c.Status = StatusRemediation
		for _, issue := range issues {
			c.Defects = append(c.Defects, DefectRecord{DefectID: "review-defect-" + issue.IssueID, CaseID: c.CaseID, LayerIndex: issue.LayerIndex, FailedRuleCodes: []string{"REVIEW_RETURNED"}, Status: DefectOpen, Source: "review", ReviewIssueID: issue.IssueID})
		}
		return nil
	}
	c.Status = StatusSealed
	c.SealedAt = &now
	return nil
}

func CanonicalDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
