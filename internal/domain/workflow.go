package domain

import (
	"fmt"
	"sort"
	"strings"
)

func BaselinePrecheck(c *ClosureCase, actor string, people []string) ([]FieldIssue, string) {
	issues := make([]FieldIssue, 0)
	checks := []struct{ field, value string }{
		{"trench_coordinates", c.TrenchCoordinates},
		{"completion_record_digest", c.CompletionRecordDigest},
		{"exposed_surface_condition", c.ExposedSurfaceCondition},
		{"frozen_by", actor},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			issues = append(issues, FieldIssue{Field: check.field, Code: "REQUIRED_FIELD", Message: "字段不能为空"})
		}
	}
	seen := map[string]int{}
	if len(people) == 0 {
		issues = append(issues, FieldIssue{Field: "responsible_people", Code: "REQUIRED_FIELD", Message: "责任人员不能为空"})
	}
	for i, person := range people {
		key := strings.TrimSpace(person)
		field := fmt.Sprintf("responsible_people[%d]", i)
		if key == "" {
			issues = append(issues, FieldIssue{Field: field, Code: "REQUIRED_FIELD", Message: "责任人员不能为空"})
			continue
		}
		if first, ok := seen[key]; ok {
			issues = append(issues, FieldIssue{Field: field, Code: "DUPLICATE_PERSON", Message: fmt.Sprintf("与 responsible_people[%d] 重复", first)})
		} else {
			seen[key] = i
		}
	}
	content := struct {
		CaseID           string   `json:"case_id"`
		SiteCode         string   `json:"site_code"`
		Coordinates      string   `json:"trench_coordinates"`
		CompletionDigest string   `json:"completion_record_digest"`
		Surface          string   `json:"exposed_surface_condition"`
		People           []string `json:"responsible_people"`
	}{c.CaseID, strings.TrimSpace(c.SiteCode), strings.TrimSpace(c.TrenchCoordinates), strings.TrimSpace(c.CompletionRecordDigest), strings.TrimSpace(c.ExposedSurfaceCondition), normalizedPeople(people)}
	return issues, CanonicalDigest(content)
}

func normalizedPeople(people []string) []string {
	out := make([]string, len(people))
	for i := range people {
		out[i] = strings.TrimSpace(people[i])
	}
	return out
}

func (c *ClosureCase) BaselineIntegrity() IntegrityStatus {
	status := IntegrityStatus{Valid: true}
	if c.Baseline == nil {
		return IntegrityStatus{Valid: false, Differences: []FieldIssue{{Field: "baseline", Code: "MISSING", Message: "冻结快照不存在"}}}
	}
	comparisons := []struct{ field, current, frozen string }{
		{"site_code", c.SiteCode, c.Baseline.SiteCode}, {"trench_coordinates", c.TrenchCoordinates, c.Baseline.TrenchCoordinates},
		{"completion_record_digest", c.CompletionRecordDigest, c.Baseline.CompletionRecordDigest}, {"exposed_surface_condition", c.ExposedSurfaceCondition, c.Baseline.ExposedSurfaceCondition},
	}
	for _, item := range comparisons {
		if item.current != item.frozen {
			status.Valid = false
			status.Differences = append(status.Differences, FieldIssue{Field: item.field, Code: "BASELINE_MISMATCH", Message: "当前值与冻结快照不一致"})
		}
	}
	if c.BaselineReceipt == nil || CanonicalDigest(*c.BaselineReceipt) != c.Baseline.ReceiptDigest {
		status.Valid = false
		status.Differences = append(status.Differences, FieldIssue{Field: "baseline_receipt", Code: "RECEIPT_MISMATCH", Message: "冻结凭证摘要不一致"})
	}
	if c.BaselineReceipt != nil {
		frozen := *c.Baseline
		frozen.ReceiptDigest = ""
		if CanonicalDigest(frozen) != CanonicalDigest(c.BaselineReceipt.NormalizedContent) {
			status.Valid = false
			status.Differences = append(status.Differences, FieldIssue{Field: "baseline_receipt.normalized_content", Code: "RECEIPT_CONTENT_MISMATCH", Message: "冻结快照与凭证规范化内容不一致"})
		}
		_, expectedSummary := BaselinePrecheck(c, c.BaselineReceipt.FrozenBy, c.BaselineReceipt.NormalizedContent.ResponsiblePeople)
		if c.BaselineReceipt.SummaryDigest != expectedSummary {
			status.Valid = false
			status.Differences = append(status.Differences, FieldIssue{Field: "baseline_receipt.summary_digest", Code: "SUMMARY_MISMATCH", Message: "凭证摘要与当前身份字段不一致"})
		}
	}
	return status
}

func MissingDraftFields(d LayerDraft, evidenceRequired bool) []string {
	missing := make([]string, 0)
	if strings.TrimSpace(d.MaterialCode) == "" {
		missing = append(missing, "material_code")
	}
	if d.ActualThicknessMM == nil {
		missing = append(missing, "actual_thickness_mm")
	}
	if d.MoisturePercent == nil {
		missing = append(missing, "moisture_percent")
	}
	if d.CompactionPercent == nil {
		missing = append(missing, "compaction_percent")
	}
	if strings.TrimSpace(d.PerformedBy) == "" {
		missing = append(missing, "performed_by")
	}
	if evidenceRequired && strings.TrimSpace(d.EvidenceDigest) == "" {
		missing = append(missing, "evidence_digest")
	}
	return missing
}

func ValidateDraftValues(d LayerDraft) error {
	if d.LayerIndex <= 0 {
		return DomainError{"LAYER_INDEX", "草稿层序号必须大于0"}
	}
	if d.ActualThicknessMM != nil && (*d.ActualThicknessMM <= 0 || *d.ActualThicknessMM > 5000) {
		return DomainError{"THICKNESS_VALUE", "实际厚度必须在1到5000毫米之间"}
	}
	if d.MoisturePercent != nil && (!finite(*d.MoisturePercent) || *d.MoisturePercent < 0 || *d.MoisturePercent > 100) {
		return DomainError{"MOISTURE_VALUE", "含水率必须在0到100之间"}
	}
	if d.CompactionPercent != nil && (!finite(*d.CompactionPercent) || *d.CompactionPercent < 0 || *d.CompactionPercent > 100) {
		return DomainError{"COMPACTION_VALUE", "压实度必须在0到100之间"}
	}
	return nil
}

func (c *ClosureCase) CheckDraft(d LayerDraft) LayerCheck {
	check := LayerCheck{Draft: d, Issues: []FieldIssue{}}
	if c.Plan == nil || d.LayerIndex < 1 || d.LayerIndex > len(c.Plan.Layers) {
		check.Issues = append(check.Issues, FieldIssue{Field: "layer_index", Code: "LAYER_ORDER", Message: "草稿层次不在批准方案中"})
		return check
	}
	check.Target = c.Plan.Layers[d.LayerIndex-1]
	for _, field := range MissingDraftFields(d, check.Target.EvidenceRequired) {
		check.Issues = append(check.Issues, FieldIssue{Field: field, Code: "REQUIRED_FIELD", Message: "正式提交前必须补齐"})
	}
	if d.LayerIndex != c.NextLayer() {
		check.Issues = append(check.Issues, FieldIssue{Field: "layer_index", Code: "LAYER_ORDER", Message: "当前应施工层已变化"})
	}
	if c.HasOpenDefect() {
		check.Issues = append(check.Issues, FieldIssue{Field: "defects", Code: "OPEN_DEFECT", Message: "存在开放缺陷"})
	}
	check.CanSubmit = len(check.Issues) == 0
	return check
}

func RequiredRetestKeys(codes []string) []string {
	keys := make([]string, 0)
	for _, code := range codes {
		key := map[string]string{"THICKNESS_OUT_OF_RANGE": "thickness_mm", "MOISTURE_OUT_OF_RANGE": "moisture_percent", "COMPACTION_BELOW_THRESHOLD": "compaction_percent"}[code]
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
