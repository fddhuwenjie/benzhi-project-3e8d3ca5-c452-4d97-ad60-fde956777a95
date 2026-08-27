package domain

import (
	"fmt"
	"math"
	"sort"
)

const RuleVersion = "backfill-rules-v1"

type RuleResult struct {
	Verdict Verdict
	Failed  []string
	Details []RuleDetail
	Digest  string
}

func EvaluateLayer(spec LayerSpec, material string, thickness int, moisture, compaction float64, evidence string) RuleResult {
	lowerThickness := float64(spec.TargetThicknessMM - spec.ThicknessToleranceMM)
	upperThickness := float64(spec.TargetThicknessMM + spec.ThicknessToleranceMM)
	moistureLower := spec.MoistureMinPercent
	moistureUpper := spec.MoistureMaxPercent
	compactionLower := spec.CompactionMinPercent
	thicknessMargin := math.Min(float64(thickness)-lowerThickness, upperThickness-float64(thickness))
	moistureMargin := math.Min(moisture-moistureLower, moistureUpper-moisture)
	compactionMargin := compaction - compactionLower
	details := []RuleDetail{
		{Code: "MATERIAL_MISMATCH", Actual: material, Requirement: spec.MaterialCode, Passed: material == spec.MaterialCode},
		{Code: "THICKNESS_OUT_OF_RANGE", Actual: fmt.Sprintf("%d", thickness), Requirement: fmt.Sprintf("%.0f..%.0f mm", lowerThickness, upperThickness), Passed: float64(thickness) >= lowerThickness && float64(thickness) <= upperThickness, LowerBound: &lowerThickness, UpperBound: &upperThickness, Margin: &thicknessMargin},
		{Code: "MOISTURE_OUT_OF_RANGE", Actual: fmt.Sprintf("%g", moisture), Requirement: fmt.Sprintf("%g..%g%%", moistureLower, moistureUpper), Passed: moisture >= moistureLower && moisture <= moistureUpper, LowerBound: &moistureLower, UpperBound: &moistureUpper, Margin: &moistureMargin},
		{Code: "COMPACTION_BELOW_THRESHOLD", Actual: fmt.Sprintf("%g", compaction), Requirement: fmt.Sprintf(">=%g%%", compactionLower), Passed: compaction >= compactionLower, LowerBound: &compactionLower, Margin: &compactionMargin},
		{Code: "EVIDENCE_REQUIRED", Actual: evidence, Requirement: map[bool]string{true: "required", false: "optional"}[spec.EvidenceRequired], Passed: !spec.EvidenceRequired || evidence != ""},
	}
	failed := make([]string, 0, 5)
	for _, detail := range details {
		if !detail.Passed {
			failed = append(failed, detail.Code)
		}
	}
	verdict := VerdictPass
	if len(failed) > 0 {
		verdict = VerdictFail
	}
	result := RuleResult{Verdict: verdict, Failed: failed, Details: details}
	result.Digest = CanonicalDigest(struct {
		Version string       `json:"version"`
		Details []RuleDetail `json:"details"`
	}{RuleVersion, details})
	return result
}

func BuildPlanReview(layers []LayerSpec) PlanReview {
	review := PlanReview{Layers: make([]PlanLayerReview, 0, len(layers))}
	for i, layer := range layers {
		review.TotalThicknessMM += layer.TargetThicknessMM
		item := PlanLayerReview{LayerIndex: layer.Index, Summary: fmt.Sprintf("%s / %d±%d mm / 含水率 %g-%g%% / 压实度 >=%g%%", layer.MaterialCode, layer.TargetThicknessMM, layer.ThicknessToleranceMM, layer.MoistureMinPercent, layer.MoistureMaxPercent, layer.CompactionMinPercent)}
		if i > 0 {
			prev := layers[i-1]
			if prev.MaterialCode != layer.MaterialCode {
				item.Changes = append(item.Changes, fmt.Sprintf("材料由 %s 变为 %s", prev.MaterialCode, layer.MaterialCode))
			}
			if prev.CompactionMinPercent != layer.CompactionMinPercent {
				item.Changes = append(item.Changes, fmt.Sprintf("压实阈值变化 %g%%", layer.CompactionMinPercent-prev.CompactionMinPercent))
			}
		}
		review.Layers = append(review.Layers, item)
		if float64(layer.ThicknessToleranceMM)/float64(layer.TargetThicknessMM) > 0.15 {
			review.Risks = append(review.Risks, PlanRisk{Code: fmt.Sprintf("LAYER_%d_TOLERANCE_RATIO", layer.Index), LayerIndex: layer.Index, Message: "厚度容差超过目标厚度的15%"})
		}
		if layer.MoistureMinPercent <= 1 || layer.MoistureMaxPercent >= 99 || layer.CompactionMinPercent >= 98 {
			review.Risks = append(review.Risks, PlanRisk{Code: fmt.Sprintf("LAYER_%d_NEAR_BOUNDARY", layer.Index), LayerIndex: layer.Index, Message: "数值接近允许边界"})
		}
		if i > 0 && layers[i-1].MaterialCode != layer.MaterialCode {
			review.Risks = append(review.Risks, PlanRisk{Code: fmt.Sprintf("LAYER_%d_MATERIAL_CHANGE", layer.Index), LayerIndex: layer.Index, Message: "连续层材料发生变化"})
		}
	}
	return review
}

func RiskCodes(review PlanReview) []string {
	codes := make([]string, 0, len(review.Risks))
	for _, risk := range review.Risks {
		codes = append(codes, risk.Code)
	}
	sort.Strings(codes)
	return codes
}

func RuleDescription(code string) string {
	return map[string]string{"MATERIAL_MISMATCH": "材料编码与方案不符", "THICKNESS_OUT_OF_RANGE": "厚度超出允许范围", "MOISTURE_OUT_OF_RANGE": "含水率超出允许范围", "COMPACTION_BELOW_THRESHOLD": "压实度低于阈值", "EVIDENCE_REQUIRED": "缺少必要证据", "REVIEW_RETURNED": "独立验收退回整改"}[code]
}

func ValidatePlan(layers []LayerSpec) error {
	if len(layers) == 0 {
		return DomainError{"EMPTY_PLAN", "至少需要一层回填方案"}
	}
	if len(layers) > 30 {
		return DomainError{"PLAN_SIZE", "回填方案不能超过30层"}
	}
	for i, l := range layers {
		if l.Index != i+1 {
			return DomainError{"LAYER_ORDER", fmt.Sprintf("层次必须从1连续编号，收到%d", l.Index)}
		}
		if err := validateIdentifier("material_code", l.MaterialCode, 120); err != nil {
			return err
		}
		if l.TargetThicknessMM <= 0 || l.TargetThicknessMM > 5000 {
			return DomainError{"PLAN_VALUE", fmt.Sprintf("第%d层目标厚度无效", l.Index)}
		}
		if l.ThicknessToleranceMM < 0 || l.ThicknessToleranceMM >= l.TargetThicknessMM {
			return DomainError{"PLAN_VALUE", fmt.Sprintf("第%d层厚度容差无效", l.Index)}
		}
		if !finite(l.MoistureMinPercent) || !finite(l.MoistureMaxPercent) || l.MoistureMinPercent < 0 || l.MoistureMaxPercent > 100 || l.MoistureMaxPercent <= l.MoistureMinPercent {
			return DomainError{"PLAN_VALUE", fmt.Sprintf("第%d层含水率范围无效", l.Index)}
		}
		if !finite(l.CompactionMinPercent) || l.CompactionMinPercent <= 0 || l.CompactionMinPercent > 100 {
			return DomainError{"PLAN_VALUE", fmt.Sprintf("第%d层压实阈值无效", l.Index)}
		}
	}
	return nil
}
