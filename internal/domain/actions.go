package domain

type Action string

const (
	ActionFreezeBaseline      Action = "freeze_baseline"
	ActionPreparePlan         Action = "prepare_plan"
	ActionApprovePlan         Action = "approve_plan"
	ActionSubmitLayer         Action = "submit_layer"
	ActionSaveDraft           Action = "save_layer_draft"
	ActionCheckDraft          Action = "check_layer_draft"
	ActionPlanRemediation     Action = "plan_remediation"
	ActionCompleteRemediation Action = "complete_remediation"
	ActionRetestDefect        Action = "retest_defect"
	ActionReview              Action = "review"
	ActionVerify              Action = "verify"
	ActionDownload            Action = "download_dossier"
)

func (c *ClosureCase) AllowedActions() []Action {
	switch c.Status {
	case StatusDraft:
		return []Action{ActionFreezeBaseline}
	case StatusFrozen:
		if !c.BaselineIntegrity().Valid {
			return nil
		}
		if c.Plan == nil {
			return []Action{ActionPreparePlan}
		}
		return []Action{ActionApprovePlan}
	case StatusPlanReady, StatusInProgress:
		return []Action{ActionSaveDraft, ActionCheckDraft, ActionSubmitLayer}
	case StatusRemediation:
		return []Action{ActionPlanRemediation, ActionCompleteRemediation, ActionRetestDefect}
	case StatusAwaitingReview:
		return []Action{ActionReview}
	case StatusSealed:
		return []Action{ActionVerify, ActionDownload}
	default:
		return nil
	}
}

func (c *ClosureCase) HasOpenDefect() bool {
	for _, defect := range c.Defects {
		if defect.Status == DefectOpen {
			return true
		}
	}
	return false
}

func (c *ClosureCase) CompletedLayers() int {
	completed := 0
	for _, execution := range c.Executions {
		if c.executionResolved(execution) {
			completed++
		}
	}
	return completed
}

func (c *ClosureCase) ReadyForReview() bool {
	if c.Plan == nil || c.HasOpenDefect() {
		return false
	}
	if len(c.Executions) != len(c.Plan.Layers) {
		return false
	}
	for _, execution := range c.Executions {
		if !c.executionResolved(execution) {
			return false
		}
	}
	return true
}

func (c *ClosureCase) executionResolved(execution LayerExecution) bool {
	if execution.Verdict == VerdictPass {
		return true
	}
	for _, defect := range c.Defects {
		if defect.LayerIndex == execution.LayerIndex && defect.Source != "review" && defect.Status == DefectOpen {
			return false
		}
	}
	return true
}

func (c *ClosureCase) ParticipantRoles() map[string][]string {
	roles := make(map[string][]string)
	if c.CreatedBy != "" {
		roles[c.CreatedBy] = append(roles[c.CreatedBy], "creator")
	}
	if c.Plan != nil {
		roles[c.Plan.PreparedBy] = append(roles[c.Plan.PreparedBy], "planner")
		if c.Plan.ApprovedBy != "" {
			roles[c.Plan.ApprovedBy] = append(roles[c.Plan.ApprovedBy], "approver")
		}
	}
	for _, execution := range c.Executions {
		roles[execution.PerformedBy] = appendUnique(roles[execution.PerformedBy], "worker")
	}
	if c.ReviewerID != "" {
		roles[c.ReviewerID] = append(roles[c.ReviewerID], "reviewer")
	}
	return roles
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
