package domain

import "fmt"

func ValidateAggregate(c *ClosureCase) error {
	if c == nil {
		return DomainError{"AGGREGATE", "案件快照不能为空"}
	}
	if err := ValidateCaseIdentity(c.CaseID); err != nil {
		return err
	}
	if c.Revision < 1 {
		return DomainError{"AGGREGATE", "案件版本必须大于0"}
	}
	if c.CreatedAt.IsZero() {
		return DomainError{"AGGREGATE", "案件缺少创建时间"}
	}
	if c.Status == StatusDraft {
		if c.Baseline != nil || c.Plan != nil || len(c.Executions) > 0 || len(c.Defects) > 0 {
			return DomainError{"AGGREGATE", "草拟案件包含不应存在的施工数据"}
		}
		return nil
	}
	if c.Baseline == nil {
		return DomainError{"AGGREGATE", "非草拟案件缺少冻结基线"}
	}
	if err := validateBaselineConsistency(c); err != nil {
		return err
	}
	if c.Status == StatusFrozen && c.Plan == nil {
		return nil
	}
	if c.Plan == nil {
		return DomainError{"AGGREGATE", "案件缺少回填方案"}
	}
	if c.Plan.CaseID != c.CaseID {
		return DomainError{"AGGREGATE", "方案案件标识不匹配"}
	}
	if err := ValidatePlan(c.Plan.Layers); err != nil {
		return err
	}
	if c.Status != StatusFrozen {
		if c.Plan.ApprovedBy == "" || c.Plan.ApprovedAt == nil {
			return DomainError{"AGGREGATE", "已施工案件的方案未完成批准"}
		}
		if c.Plan.PreparedBy == c.Plan.ApprovedBy {
			return DomainError{"AGGREGATE", "方案签署职责未分离"}
		}
	}
	if len(c.Executions) > len(c.Plan.Layers) {
		return DomainError{"AGGREGATE", "施工层数量超过批准方案"}
	}
	for index, execution := range c.Executions {
		if execution.CaseID != c.CaseID {
			return DomainError{"AGGREGATE", fmt.Sprintf("第%d项施工记录案件标识不匹配", index+1)}
		}
		if execution.LayerIndex != index+1 {
			return DomainError{"AGGREGATE", "施工记录层序不连续"}
		}
		if execution.Verdict != VerdictPass && execution.Verdict != VerdictFail {
			return DomainError{"AGGREGATE", "施工记录判定无效"}
		}
	}
	for _, defect := range c.Defects {
		if defect.CaseID != c.CaseID || defect.LayerIndex < 1 || defect.LayerIndex > len(c.Plan.Layers) {
			return DomainError{"AGGREGATE", "缺陷关联的案件或层次无效"}
		}
		if len(defect.FailedRuleCodes) == 0 {
			return DomainError{"AGGREGATE", "缺陷缺少失败规则"}
		}
		if defect.Status == DefectClosed && (defect.ClosedAt == nil || defect.ClosedBy == "") {
			return DomainError{"AGGREGATE", "关闭缺陷缺少复验签署"}
		}
	}
	return validateTerminalConsistency(c)
}

func validateBaselineConsistency(c *ClosureCase) error {
	baseline := c.Baseline
	if baseline.SiteCode != c.SiteCode || baseline.TrenchCoordinates != c.TrenchCoordinates || baseline.CompletionRecordDigest != c.CompletionRecordDigest || baseline.ExposedSurfaceCondition != c.ExposedSurfaceCondition {
		return DomainError{"BASELINE_TAMPERED", "案件字段与冻结基线不一致"}
	}
	if baseline.FrozenAt.IsZero() {
		return DomainError{"AGGREGATE", "冻结基线缺少时间"}
	}
	return ValidateResponsiblePeople(baseline.ResponsiblePeople)
}

func validateTerminalConsistency(c *ClosureCase) error {
	if c.Status == StatusSealed {
		if c.SealedAt == nil || c.ReviewerID == "" || c.ReviewDecision != "pass" {
			return DomainError{"AGGREGATE", "封存案件缺少有效验收签署"}
		}
		if c.HasOpenDefect() {
			return DomainError{"AGGREGATE", "封存案件仍有开放缺陷"}
		}
		if !c.ReadyForReview() {
			return DomainError{"AGGREGATE", "封存案件并非全部层次合格"}
		}
	} else if c.SealedAt != nil {
		return DomainError{"AGGREGATE", "非封存案件包含封存时间"}
	}
	return nil
}
