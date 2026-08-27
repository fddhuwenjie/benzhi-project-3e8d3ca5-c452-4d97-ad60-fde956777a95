package domain

import (
	"errors"
	"fmt"
	"time"
)

type CaseStatus string

const (
	StatusDraft          CaseStatus = "draft"
	StatusFrozen         CaseStatus = "baseline_frozen"
	StatusPlanReady      CaseStatus = "plan_approved"
	StatusInProgress     CaseStatus = "in_progress"
	StatusRemediation    CaseStatus = "remediation"
	StatusAwaitingReview CaseStatus = "awaiting_review"
	StatusSealed         CaseStatus = "sealed"
)

type Verdict string

const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
)

type DefectStatus string

const (
	DefectOpen   DefectStatus = "open"
	DefectClosed DefectStatus = "closed"
)

type LayerSpec struct {
	Index                int     `json:"index"`
	MaterialCode         string  `json:"material_code"`
	TargetThicknessMM    int     `json:"target_thickness_mm"`
	ThicknessToleranceMM int     `json:"thickness_tolerance_mm"`
	MoistureMinPercent   float64 `json:"moisture_min_percent"`
	MoistureMaxPercent   float64 `json:"moisture_max_percent"`
	CompactionMinPercent float64 `json:"compaction_min_percent"`
	EvidenceRequired     bool    `json:"evidence_required"`
}

type Baseline struct {
	SiteCode                string    `json:"site_code"`
	TrenchCoordinates       string    `json:"trench_coordinates"`
	CompletionRecordDigest  string    `json:"completion_record_digest"`
	ExposedSurfaceCondition string    `json:"exposed_surface_condition"`
	ResponsiblePeople       []string  `json:"responsible_people"`
	FrozenAt                time.Time `json:"frozen_at"`
	FrozenBy                string    `json:"frozen_by"`
	ReceiptDigest           string    `json:"receipt_digest"`
}

type FieldIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BaselineReceipt struct {
	CaseID            string    `json:"case_id"`
	NormalizedContent Baseline  `json:"normalized_content"`
	FrozenBy          string    `json:"frozen_by"`
	FrozenAt          time.Time `json:"frozen_at"`
	SummaryDigest     string    `json:"summary_digest"`
}

type IntegrityStatus struct {
	Valid       bool         `json:"valid"`
	Differences []FieldIssue `json:"differences"`
}

type PlanLayerReview struct {
	LayerIndex int      `json:"layer_index"`
	Summary    string   `json:"summary"`
	Changes    []string `json:"changes,omitempty"`
}

type PlanRisk struct {
	Code       string `json:"code"`
	LayerIndex int    `json:"layer_index"`
	Message    string `json:"message"`
}

type PlanReview struct {
	TotalThicknessMM int               `json:"total_thickness_mm"`
	Layers           []PlanLayerReview `json:"layers"`
	Risks            []PlanRisk        `json:"risks"`
}

type BackfillPlan struct {
	PlanID             string      `json:"plan_id"`
	CaseID             string      `json:"case_id"`
	PreparedBy         string      `json:"prepared_by"`
	ApprovedBy         string      `json:"approved_by"`
	ApprovedAt         *time.Time  `json:"approved_at,omitempty"`
	Layers             []LayerSpec `json:"layers"`
	PlanDigest         string      `json:"plan_digest"`
	Review             PlanReview  `json:"review"`
	ConfirmedRiskCodes []string    `json:"confirmed_risk_codes,omitempty"`
	Revision           int64       `json:"revision"`
}

type RuleDetail struct {
	Code        string   `json:"code"`
	Actual      string   `json:"actual"`
	Requirement string   `json:"requirement"`
	Passed      bool     `json:"passed"`
	LowerBound  *float64 `json:"lower_bound,omitempty"`
	UpperBound  *float64 `json:"upper_bound,omitempty"`
	Margin      *float64 `json:"margin,omitempty"`
}

type EvaluationSnapshot struct {
	RuleVersion  string       `json:"rule_version"`
	PlanDigest   string       `json:"plan_digest"`
	Rules        []RuleDetail `json:"rules"`
	ResultDigest string       `json:"result_digest"`
}

type LayerExecution struct {
	ExecutionID       string             `json:"execution_id"`
	CaseID            string             `json:"case_id"`
	LayerIndex        int                `json:"layer_index"`
	MaterialCode      string             `json:"material_code"`
	ActualThicknessMM int                `json:"actual_thickness_mm"`
	MoisturePercent   float64            `json:"moisture_percent"`
	CompactionPercent float64            `json:"compaction_percent"`
	PerformedBy       string             `json:"performed_by"`
	EvidenceDigest    string             `json:"evidence_digest"`
	SubmittedAt       time.Time          `json:"submitted_at"`
	Verdict           Verdict            `json:"verdict"`
	FailedRuleCodes   []string           `json:"failed_rule_codes,omitempty"`
	Evaluation        EvaluationSnapshot `json:"evaluation"`
}

type LayerDraft struct {
	CaseID            string    `json:"case_id"`
	LayerIndex        int       `json:"layer_index"`
	MaterialCode      string    `json:"material_code,omitempty"`
	ActualThicknessMM *int      `json:"actual_thickness_mm,omitempty"`
	MoisturePercent   *float64  `json:"moisture_percent,omitempty"`
	CompactionPercent *float64  `json:"compaction_percent,omitempty"`
	PerformedBy       string    `json:"performed_by,omitempty"`
	EvidenceDigest    string    `json:"evidence_digest,omitempty"`
	DraftVersion      int64     `json:"draft_version"`
	UpdatedAt         time.Time `json:"updated_at"`
	MissingFields     []string  `json:"missing_fields"`
}

type LayerCheck struct {
	Draft     LayerDraft   `json:"draft"`
	Target    LayerSpec    `json:"target"`
	Issues    []FieldIssue `json:"issues"`
	CanSubmit bool         `json:"can_submit"`
}

type RemediationPlan struct {
	Version       int64     `json:"version"`
	CauseCategory string    `json:"cause_category"`
	Cause         string    `json:"cause"`
	Action        string    `json:"corrective_action"`
	ResponsibleBy string    `json:"responsible_by"`
	PlannedAt     time.Time `json:"planned_completion_at"`
	RecordedBy    string    `json:"recorded_by"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type RemediationCompletion struct {
	PlanVersion    int64     `json:"plan_version"`
	Description    string    `json:"description"`
	EvidenceDigest string    `json:"evidence_digest"`
	CompletedBy    string    `json:"completed_by"`
	CompletedAt    time.Time `json:"completed_at"`
}

type RetestAttempt struct {
	AttemptedBy    string             `json:"attempted_by"`
	AttemptedAt    time.Time          `json:"attempted_at"`
	EvidenceDigest string             `json:"evidence_digest"`
	Values         map[string]float64 `json:"values"`
	RuleResults    []RuleDetail       `json:"rule_results"`
	RemainingRules []string           `json:"remaining_rules"`
	Passed         bool               `json:"passed"`
}

type DefectRecord struct {
	DefectID         string                 `json:"defect_id"`
	CaseID           string                 `json:"case_id"`
	LayerIndex       int                    `json:"layer_index"`
	FailedRuleCodes  []string               `json:"failed_rule_codes"`
	Cause            string                 `json:"cause"`
	CorrectiveAction string                 `json:"corrective_action"`
	EvidenceDigest   string                 `json:"evidence_digest"`
	RetestValues     map[string]float64     `json:"retest_values,omitempty"`
	Status           DefectStatus           `json:"status"`
	ClosedBy         string                 `json:"closed_by,omitempty"`
	ClosedAt         *time.Time             `json:"closed_at,omitempty"`
	Source           string                 `json:"source"`
	ReviewIssueID    string                 `json:"review_issue_id,omitempty"`
	Plans            []RemediationPlan      `json:"plans,omitempty"`
	Completion       *RemediationCompletion `json:"completion,omitempty"`
	RetestAttempts   []RetestAttempt        `json:"retest_attempts,omitempty"`
}

type ReviewIssue struct {
	IssueID              string `json:"issue_id"`
	LayerIndex           int    `json:"layer_index"`
	Description          string `json:"description"`
	RequiredAction       string `json:"required_action"`
	RequiredEvidenceType string `json:"required_evidence_type"`
}

type ReviewRound struct {
	Round      int           `json:"round"`
	ReviewerID string        `json:"reviewer_id"`
	Decision   string        `json:"decision"`
	Issues     []ReviewIssue `json:"issues,omitempty"`
	ReviewedAt time.Time     `json:"reviewed_at"`
}

type ClosureCase struct {
	CaseID                  string           `json:"case_id"`
	SiteCode                string           `json:"site_code"`
	TrenchCoordinates       string           `json:"trench_coordinates"`
	CompletionRecordDigest  string           `json:"completion_record_digest"`
	ExposedSurfaceCondition string           `json:"exposed_surface_condition"`
	Status                  CaseStatus       `json:"status"`
	Revision                int64            `json:"revision"`
	CreatedBy               string           `json:"created_by"`
	CreatedAt               time.Time        `json:"created_at"`
	SealedAt                *time.Time       `json:"sealed_at,omitempty"`
	Baseline                *Baseline        `json:"baseline,omitempty"`
	BaselineReceipt         *BaselineReceipt `json:"baseline_receipt,omitempty"`
	Plan                    *BackfillPlan    `json:"plan,omitempty"`
	Executions              []LayerExecution `json:"executions,omitempty"`
	Defects                 []DefectRecord   `json:"defects,omitempty"`
	ReviewerID              string           `json:"reviewer_id,omitempty"`
	ReviewDecision          string           `json:"review_decision,omitempty"`
	ReviewHistory           []ReviewRound    `json:"review_history,omitempty"`
}

type Event struct {
	Sequence       int64          `json:"sequence"`
	CaseID         string         `json:"case_id"`
	Revision       int64          `json:"revision"`
	Type           string         `json:"type"`
	At             time.Time      `json:"at"`
	Actor          string         `json:"actor"`
	Payload        map[string]any `json:"payload"`
	PreviousDigest string         `json:"previous_digest"`
	Digest         string         `json:"digest"`
}

var (
	ErrNotFound      = errors.New("case not found")
	ErrAlreadyExists = errors.New("case already exists")
	ErrConflict      = errors.New("revision conflict")
	ErrInvalid       = errors.New("invalid request")
	ErrForbidden     = errors.New("forbidden transition")
	ErrSealed        = errors.New("case sealed")
)

type DomainError struct {
	Code    string
	Message string
}

func (e DomainError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
