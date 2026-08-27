package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func (c *ClosureCase) findOpenDefect(id string) (*DefectRecord, error) {
	if c.Status != StatusRemediation {
		return nil, DomainError{"STATE", "案件当前不在整改状态"}
	}
	for i := range c.Defects {
		if c.Defects[i].DefectID == id {
			if c.Defects[i].Status != DefectOpen {
				return nil, DomainError{"DEFECT_STATE", "缺陷已关闭"}
			}
			return &c.Defects[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *ClosureCase) RecordRemediationPlan(id, category, cause, action, responsible string, planned time.Time, actor string, now time.Time) error {
	d, err := c.findOpenDefect(id)
	if err != nil {
		return err
	}
	for name, value := range map[string]string{"原因分类": category, "原因说明": cause, "纠正措施": action, "责任人": responsible, "登记人": actor} {
		if strings.TrimSpace(value) == "" {
			return DomainError{"REMEDIATION_FIELDS", name + "不能为空"}
		}
	}
	if planned.IsZero() {
		return DomainError{"REMEDIATION_FIELDS", "计划完成时间不能为空"}
	}
	d.Plans = append(d.Plans, RemediationPlan{Version: int64(len(d.Plans) + 1), CauseCategory: category, Cause: cause, Action: action, ResponsibleBy: responsible, PlannedAt: planned.UTC(), RecordedBy: actor, RecordedAt: now.UTC()})
	d.Cause, d.CorrectiveAction = cause, action
	c.Revision++
	return nil
}

func (c *ClosureCase) CompleteRemediation(id, description, evidence, actor string, now time.Time) error {
	d, err := c.findOpenDefect(id)
	if err != nil {
		return err
	}
	if len(d.Plans) == 0 {
		return DomainError{"REMEDIATION_STAGE", "必须先登记整改方案"}
	}
	if strings.TrimSpace(description) == "" || strings.TrimSpace(evidence) == "" || strings.TrimSpace(actor) == "" {
		return DomainError{"REMEDIATION_FIELDS", "完成说明、证据摘要和完成人不能为空"}
	}
	version := d.Plans[len(d.Plans)-1].Version
	d.Completion = &RemediationCompletion{PlanVersion: version, Description: description, EvidenceDigest: evidence, CompletedBy: actor, CompletedAt: now.UTC()}
	d.EvidenceDigest = evidence
	c.Revision++
	return nil
}

func requiredValueKey(code string) string {
	return map[string]string{
		"MATERIAL_MISMATCH": "material_confirmed", "THICKNESS_OUT_OF_RANGE": "thickness_mm", "MOISTURE_OUT_OF_RANGE": "moisture_percent",
		"COMPACTION_BELOW_THRESHOLD": "compaction_percent", "EVIDENCE_REQUIRED": "evidence_confirmed", "REVIEW_RETURNED": "review_confirmed",
	}[code]
}

func (c *ClosureCase) RetestCompletedDefect(id, actor, evidence string, values map[string]float64, now time.Time) (bool, []string, error) {
	d, err := c.findOpenDefect(id)
	if err != nil {
		return false, nil, err
	}
	if d.Completion == nil {
		return false, nil, DomainError{"REMEDIATION_STAGE", "纠正措施尚未完成"}
	}
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(evidence) == "" {
		return false, nil, DomainError{"RETEST_FIELDS", "复验人员和证据不能为空"}
	}
	if value, ok := values["thickness_mm"]; ok && (!finite(value) || value < 1 || value > 5000 || math.Trunc(value) != value) {
		return false, nil, DomainError{"RETEST_VALUES", "复验厚度必须为1到5000之间的整数"}
	}
	if value, ok := values["moisture_percent"]; ok && (!finite(value) || value < 0 || value > 100) {
		return false, nil, DomainError{"RETEST_VALUES", "复验含水率必须在0到100之间"}
	}
	if value, ok := values["compaction_percent"]; ok && (!finite(value) || value < 0 || value > 100) {
		return false, nil, DomainError{"RETEST_VALUES", "复验压实度必须在0到100之间"}
	}
	for _, code := range d.FailedRuleCodes {
		key := requiredValueKey(code)
		if key != "" {
			if _, ok := values[key]; !ok {
				return false, nil, DomainError{"RETEST_COVERAGE", "缺少原失败规则的复验输入: " + code}
			}
		}
	}
	spec := c.Plan.Layers[d.LayerIndex-1]
	var original *LayerExecution
	for i := range c.Executions {
		if c.Executions[i].LayerIndex == d.LayerIndex {
			original = &c.Executions[i]
			break
		}
	}
	material, thickness, moisture, compaction := spec.MaterialCode, spec.TargetThicknessMM, (spec.MoistureMinPercent+spec.MoistureMaxPercent)/2, spec.CompactionMinPercent
	if original != nil {
		material, thickness, moisture, compaction = original.MaterialCode, original.ActualThicknessMM, original.MoisturePercent, original.CompactionPercent
	}
	if values["material_confirmed"] == 1 {
		material = spec.MaterialCode
	}
	if v, ok := values["thickness_mm"]; ok {
		thickness = int(v)
	}
	if v, ok := values["moisture_percent"]; ok {
		moisture = v
	}
	if v, ok := values["compaction_percent"]; ok {
		compaction = v
	}
	result := EvaluateLayer(spec, material, thickness, moisture, compaction, evidence)
	byCode := map[string]RuleDetail{}
	for _, detail := range result.Details {
		byCode[detail.Code] = detail
	}
	remaining := make([]string, 0)
	targeted := make([]RuleDetail, 0, len(d.FailedRuleCodes))
	for _, code := range d.FailedRuleCodes {
		detail, ok := byCode[code]
		if code == "REVIEW_RETURNED" {
			detail = RuleDetail{Code: code, Actual: fmt.Sprintf("%g", values["review_confirmed"]), Requirement: "=1", Passed: values["review_confirmed"] == 1}
			ok = true
		}
		if code == "EVIDENCE_REQUIRED" {
			detail.Passed = values["evidence_confirmed"] == 1 && evidence != ""
		}
		if !ok || !detail.Passed {
			remaining = append(remaining, code)
		}
		targeted = append(targeted, detail)
	}
	sort.Strings(remaining)
	passed := len(remaining) == 0
	d.RetestAttempts = append(d.RetestAttempts, RetestAttempt{AttemptedBy: actor, AttemptedAt: now.UTC(), EvidenceDigest: evidence, Values: cloneValues(values), RuleResults: targeted, RemainingRules: remaining, Passed: passed})
	if passed {
		d.RetestValues, d.EvidenceDigest, d.Status, d.ClosedBy = cloneValues(values), evidence, DefectClosed, actor
		closed := now.UTC()
		d.ClosedAt = &closed
		if !c.HasOpenDefect() {
			c.Status = StatusInProgress
			if c.ReadyForReview() {
				c.Status = StatusAwaitingReview
			}
		}
	}
	c.Revision++
	return passed, remaining, nil
}

func cloneValues(values map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
