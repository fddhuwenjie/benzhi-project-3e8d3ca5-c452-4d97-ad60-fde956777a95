package evidence

import (
	"encoding/json"
	"fmt"
	"time"

	"siteclosure/internal/domain"
)

type ClosureDossier struct {
	DossierID        string          `json:"dossier_id"`
	CaseID           string          `json:"case_id"`
	FinalRevision    int64           `json:"final_revision"`
	ReviewerID       string          `json:"reviewer_id"`
	Decision         string          `json:"decision"`
	CanonicalPayload json.RawMessage `json:"canonical_payload"`
	EventChainHead   string          `json:"event_chain_head"`
	SHA256Digest     string          `json:"sha256_digest"`
	SealedAt         time.Time       `json:"sealed_at"`
}
type Verification struct {
	Valid          bool     `json:"valid"`
	DossierDigest  string   `json:"dossier_digest"`
	EventChainHead string   `json:"event_chain_head"`
	Checks         []string `json:"checks"`
	Error          string   `json:"error,omitempty"`
}
type CheckResult struct {
	Code          string `json:"code"`
	Passed        bool   `json:"passed"`
	Message       string `json:"message"`
	EventSequence int64  `json:"event_sequence,omitempty"`
}
type VerificationReport struct {
	Valid          bool          `json:"valid"`
	CaseID         string        `json:"case_id"`
	DossierDigest  string        `json:"dossier_digest"`
	EventChainHead string        `json:"event_chain_head"`
	CheckedAt      time.Time     `json:"checked_at"`
	Checks         []CheckResult `json:"checks"`
}
type Service struct{}

func New() *Service { return &Service{} }

type payload struct {
	Case   *domain.ClosureCase `json:"case"`
	Events []domain.Event      `json:"events"`
}

func (s *Service) Build(c *domain.ClosureCase, events []domain.Event) (ClosureDossier, error) {
	if c.Status != domain.StatusSealed || c.SealedAt == nil {
		return ClosureDossier{}, fmt.Errorf("only sealed case can build dossier")
	}
	if err := verifyChain(events); err != nil {
		return ClosureDossier{}, err
	}
	b, err := json.Marshal(payload{c, events})
	if err != nil {
		return ClosureDossier{}, err
	}
	head := ""
	if len(events) > 0 {
		head = events[len(events)-1].Digest
	}
	digest := domain.CanonicalDigest(json.RawMessage(b))
	return ClosureDossier{DossierID: "dossier-" + c.CaseID, CaseID: c.CaseID, FinalRevision: c.Revision, ReviewerID: c.ReviewerID, Decision: c.ReviewDecision, CanonicalPayload: b, EventChainHead: head, SHA256Digest: digest, SealedAt: *c.SealedAt}, nil
}

func (s *Service) Verify(c *domain.ClosureCase, events []domain.Event) Verification {
	checks := []string{}
	if err := verifyChain(events); err != nil {
		return Verification{Valid: false, Error: err.Error()}
	}
	checks = append(checks, "事件序号连续", "前序摘要链完整")
	d, err := s.Build(c, events)
	if err != nil {
		return Verification{Valid: false, Error: err.Error(), Checks: checks}
	}
	if c.ReviewerID == c.Plan.PreparedBy || c.ReviewerID == c.Plan.ApprovedBy || c.WorkerIDs()[c.ReviewerID] {
		return Verification{Valid: false, Error: "验收签署职责未分离", Checks: checks}
	}
	checks = append(checks, "终态档案摘要可重复", "验收签署职责分离")
	return Verification{Valid: true, DossierDigest: d.SHA256Digest, EventChainHead: d.EventChainHead, Checks: checks}
}

func (s *Service) Report(c *domain.ClosureCase, events []domain.Event, stored ClosureDossier, checkedAt time.Time) VerificationReport {
	report := VerificationReport{CaseID: c.CaseID, DossierDigest: stored.SHA256Digest, EventChainHead: stored.EventChainHead, CheckedAt: checkedAt.UTC(), Checks: []CheckResult{}}
	add := func(code string, passed bool, message string) {
		report.Checks = append(report.Checks, CheckResult{Code: code, Passed: passed, Message: message})
	}
	add("BASELINE_PRESENT", c.Baseline != nil && c.BaselineReceipt != nil, "冻结基线与冻结凭证齐全")
	add("BASELINE_INTEGRITY", c.BaselineIntegrity().Valid, "案件身份字段与冻结快照一致")
	planOK := c.Plan != nil && c.Plan.ApprovedAt != nil && c.Plan.PlanDigest != ""
	add("APPROVED_PLAN", planOK, "批准方案及签署齐全")
	layersOK := planOK && len(c.Executions) == len(c.Plan.Layers)
	if planOK {
		for index := range c.Plan.Layers {
			passed := index < len(c.Executions)
			if passed {
				ex := c.Executions[index]
				passed = ex.LayerIndex == index+1 && ex.Evaluation.PlanDigest == c.Plan.PlanDigest && ex.Evaluation.RuleVersion != "" && ex.Evaluation.ResultDigest != ""
			}
			add(fmt.Sprintf("LAYER_EXECUTION_%02d", index+1), passed, fmt.Sprintf("第%d层实绩与提交时判定快照完整", index+1))
			if !passed {
				layersOK = false
			}
		}
	}
	add("LAYER_EXECUTIONS", layersOK, "全部逐层实绩检查完成")
	defectsOK := !c.HasOpenDefect()
	add("DEFECT_CLOSURE", defectsOK, "全部缺陷已闭环")
	reviewOK := c.ReviewDecision == "pass" && c.ReviewerID != "" && len(c.ReviewHistory) > 0 && c.ReviewHistory[len(c.ReviewHistory)-1].ReviewerID == c.ReviewerID && c.ReviewHistory[len(c.ReviewHistory)-1].Decision == "pass"
	add("REVIEW_SIGNATURE", reviewOK, "终态验收签署与最后验收轮次一致")
	dutyOK := c.Plan != nil && c.ReviewerID != c.Plan.PreparedBy && c.ReviewerID != c.Plan.ApprovedBy && !c.WorkerIDs()[c.ReviewerID]
	add("REVIEW_DUTY_SEPARATION", dutyOK, "验收员与编制、批准及全部施工人员分离")
	reviewEventOK := false
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Type == "review_completed" {
			reviewEventOK = events[index].Actor == c.ReviewerID && events[index].Revision == c.Revision
			break
		}
	}
	add("REVIEW_EVENT", reviewEventOK, "终态验收签署与验收事件一致")
	chainOK := verifyChain(events) == nil
	add("EVENT_CONTINUITY", chainOK, "事件序号、摘要及前序关系连续")
	head := ""
	if len(events) > 0 {
		head = events[len(events)-1].Digest
	}
	add("CHAIN_HEAD", head != "" && stored.EventChainHead == head, "档案链头与事件链终点一致")
	identityOK := c.Status == domain.StatusSealed && c.SealedAt != nil && stored.CaseID == c.CaseID && stored.FinalRevision == c.Revision && stored.ReviewerID == c.ReviewerID && stored.SealedAt.Equal(*c.SealedAt)
	add("TERMINAL_IDENTITY", identityOK, "case_id、final_revision、sealed_at 和 reviewer_id 与终态一致")
	digestOK := stored.SHA256Digest == domain.CanonicalDigest(json.RawMessage(stored.CanonicalPayload))
	add("DOSSIER_DIGEST", digestOK, "档案总摘要与规范化载荷一致")
	expected, currentErr := s.Build(c, events)
	add("DOSSIER_CURRENT_MATCH", currentErr == nil && expected.SHA256Digest == stored.SHA256Digest, "已保存档案与终态聚合及完整事件序列一致")
	var archived payload
	payloadOK := json.Unmarshal(stored.CanonicalPayload, &archived) == nil && archived.Case != nil && archived.Case.CaseID == stored.CaseID
	add("DOSSIER_PAYLOAD", payloadOK, "档案载荷可解析且案件标识一致")
	report.Valid = true
	for _, check := range report.Checks {
		if !check.Passed {
			report.Valid = false
			break
		}
	}
	return report
}
func verifyChain(es []domain.Event) error {
	if len(es) == 0 {
		return fmt.Errorf("事件序列不能为空")
	}
	return domain.ValidateEventChain(es[0].CaseID, es)
}
