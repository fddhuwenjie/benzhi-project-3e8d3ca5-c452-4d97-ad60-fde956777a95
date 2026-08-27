package domain

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateCaseIdentity(caseID string) error {
	return validateIdentifier("case_id", caseID, 128)
}

func ValidateActor(actor string) error {
	return validateText("责任人员", actor, 1, 120)
}

func ValidateDigest(name, digest string) error {
	if err := validateText(name, digest, 1, 512); err != nil {
		return err
	}
	for _, r := range digest {
		if unicode.IsControl(r) {
			return DomainError{"DIGEST_FORMAT", name + "不能包含控制字符"}
		}
	}
	return nil
}

func ValidateNewCaseFields(caseID, siteCode, coordinates, completionDigest, surface, actor string) error {
	if err := ValidateCaseIdentity(caseID); err != nil {
		return err
	}
	if err := validateIdentifier("site_code", siteCode, 80); err != nil {
		return err
	}
	if err := validateText("探方坐标", coordinates, 1, 240); err != nil {
		return err
	}
	if err := ValidateDigest("完成记录摘要", completionDigest); err != nil {
		return err
	}
	if err := validateText("暴露面状态", surface, 1, 2000); err != nil {
		return err
	}
	return ValidateActor(actor)
}

func ValidateResponsiblePeople(people []string) error {
	if len(people) == 0 {
		return DomainError{"BASELINE_FIELDS", "责任人员不能为空"}
	}
	if len(people) > 30 {
		return DomainError{"BASELINE_FIELDS", "责任人员数量不能超过30人"}
	}
	seen := make(map[string]struct{}, len(people))
	for _, person := range people {
		if err := ValidateActor(person); err != nil {
			return err
		}
		key := strings.TrimSpace(person)
		if _, exists := seen[key]; exists {
			return DomainError{"BASELINE_FIELDS", "责任人员不能重复"}
		}
		seen[key] = struct{}{}
	}
	return nil
}

func ValidateExecutionInput(execution LayerExecution) error {
	if err := validateIdentifier("execution_id", execution.ExecutionID, 128); err != nil {
		return err
	}
	if err := ValidateCaseIdentity(execution.CaseID); err != nil {
		return err
	}
	if execution.LayerIndex <= 0 {
		return DomainError{"LAYER_INDEX", "施工层序号必须大于0"}
	}
	if err := validateIdentifier("material_code", execution.MaterialCode, 120); err != nil {
		return err
	}
	if execution.ActualThicknessMM <= 0 || execution.ActualThicknessMM > 5000 {
		return DomainError{"THICKNESS_VALUE", "实际厚度必须在1到5000毫米之间"}
	}
	if !finite(execution.MoisturePercent) || execution.MoisturePercent < 0 || execution.MoisturePercent > 100 {
		return DomainError{"MOISTURE_VALUE", "含水率必须在0到100之间"}
	}
	if !finite(execution.CompactionPercent) || execution.CompactionPercent < 0 || execution.CompactionPercent > 100 {
		return DomainError{"COMPACTION_VALUE", "压实度必须在0到100之间"}
	}
	if err := ValidateActor(execution.PerformedBy); err != nil {
		return err
	}
	if err := ValidateDigest("施工证据摘要", execution.EvidenceDigest); err != nil {
		return err
	}
	return nil
}

func ValidateRetestValues(values map[string]float64) error {
	if values == nil {
		return DomainError{"RETEST_VALUES", "复验数据不能为空"}
	}
	thickness, thicknessOK := values["thickness_mm"]
	moisture, moistureOK := values["moisture_percent"]
	compaction, compactionOK := values["compaction_percent"]
	if !thicknessOK || !moistureOK || !compactionOK {
		return DomainError{"RETEST_VALUES", "复验必须包含厚度、含水率和压实度"}
	}
	if !finite(thickness) || thickness <= 0 || thickness > 5000 || math.Trunc(thickness) != thickness {
		return DomainError{"RETEST_VALUES", "复验厚度必须为1到5000之间的整数"}
	}
	if !finite(moisture) || moisture < 0 || moisture > 100 {
		return DomainError{"RETEST_VALUES", "复验含水率必须在0到100之间"}
	}
	if !finite(compaction) || compaction < 0 || compaction > 100 {
		return DomainError{"RETEST_VALUES", "复验压实度必须在0到100之间"}
	}
	return nil
}

func ValidateRemediation(cause, correctiveAction, evidence, actor string) error {
	if err := validateText("缺陷原因", cause, 1, 2000); err != nil {
		return err
	}
	if err := validateText("纠正措施", correctiveAction, 1, 2000); err != nil {
		return err
	}
	if err := ValidateDigest("整改证据摘要", evidence); err != nil {
		return err
	}
	return ValidateActor(actor)
}

func validateIdentifier(name, value string, maxRunes int) error {
	if err := validateText(name, value, 1, maxRunes); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return DomainError{"IDENTIFIER_FORMAT", fmt.Sprintf("%s包含不允许的字符", name)}
		}
	}
	if value == "." || value == ".." {
		return DomainError{"IDENTIFIER_FORMAT", fmt.Sprintf("%s格式无效", name)}
	}
	return nil
}

func validateText(name, value string, minRunes, maxRunes int) error {
	trimmed := strings.TrimSpace(value)
	length := utf8.RuneCountInString(trimmed)
	if length < minRunes {
		return DomainError{"REQUIRED_FIELD", name + "不能为空"}
	}
	if length > maxRunes {
		return DomainError{"FIELD_TOO_LONG", fmt.Sprintf("%s不能超过%d个字符", name, maxRunes)}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
