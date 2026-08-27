package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"siteclosure/internal/domain"
	"siteclosure/internal/evidence"
	"siteclosure/internal/storage"
)

type Service struct {
	store    *storage.Store
	locks    sync.Map
	evidence *evidence.Service
}

func New(store *storage.Store) *Service { return &Service{store: store, evidence: evidence.New()} }
func (s *Service) lock(id string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	s.locks.CompareAndDelete(id, m)
	return m
}
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) guard(reqID string, payload any) (json.RawMessage, bool, error) {
	if reqID == "" {
		return nil, false, domain.DomainError{Code: "REQUEST_ID", Message: "request_id不能为空"}
	}
	fp := fingerprint(payload)
	if r, ok := s.store.GetIdem(reqID); ok {
		if r.Fingerprint != fp {
			return nil, false, domain.DomainError{Code: "IDEMPOTENCY", Message: "相同request_id不能提交不同载荷"}
		}
		return r.Response, true, nil
	}
	return nil, false, nil
}
func (s *Service) saveResponse(reqID string, payload any, v any) error {
	b, _ := json.Marshal(v)
	return s.store.PutIdem(reqID, storage.IdempotentResult{Fingerprint: fingerprint(payload), Response: b})
}
func (s *Service) mutate(id, reqID string, payload any, expected int64, eventType string, fn func(*domain.ClosureCase) (string, error)) (any, error) {
	return s.mutateOptions(id, reqID, payload, expected, eventType, false, fn)
}
func (s *Service) mutateOptions(id, reqID string, payload any, expected int64, eventType string, deleteDraft bool, fn func(*domain.ClosureCase) (string, error)) (any, error) {
	m := s.lock(id)
	defer m.Unlock()
	if cached, ok, err := s.guard(reqID, payload); err != nil {
		return nil, err
	} else if ok {
		return cached, nil
	}
	c, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if expected > 0 && c.Revision != expected {
		return nil, domain.ErrConflict
	}
	actor, err := fn(c)
	if err != nil {
		return nil, err
	}
	eventPayload := map[string]any{"request_id": reqID, "state_digest": domain.CanonicalDigest(c)}
	switch eventType {
	case "baseline_frozen":
		eventPayload["baseline_receipt"] = c.BaselineReceipt
	case "plan_prepared", "plan_approved":
		eventPayload["plan"] = c.Plan
	case "layer_submitted":
		if len(c.Executions) > 0 {
			eventPayload["execution"] = c.Executions[len(c.Executions)-1]
		}
	case "review_completed":
		if len(c.ReviewHistory) > 0 {
			eventPayload["review"] = c.ReviewHistory[len(c.ReviewHistory)-1]
		}
	default:
		eventPayload["defects"] = c.Defects
	}
	ev, _ := s.store.AppendEvent(c, eventType, actor, eventPayload, time.Now().Unix())
	resp := map[string]any{"case": c, "event": ev}
	responseBytes, _ := json.Marshal(resp)
	commit := storage.IdempotentResult{Fingerprint: fingerprint(payload), Response: responseBytes}
	var commitErr error
	if deleteDraft {
		commitErr = s.store.CommitAndDeleteDraft(c, ev, reqID, commit)
	} else {
		commitErr = s.store.Commit(c, ev, reqID, commit)
	}
	if commitErr != nil {
		return nil, commitErr
	}
	return resp, nil
}

type CreateCaseRequest struct{ RequestID, CaseID, SiteCode, Coordinates, CompletionDigest, Surface, Actor string }

func (s *Service) CreateCase(r CreateCaseRequest) (any, error) {
	m := s.lock(r.CaseID)
	defer m.Unlock()
	if x, ok, err := s.guard(r.RequestID, r); ok || err != nil {
		return x, err
	}
	if _, err := s.store.Get(r.CaseID); err == nil {
		return nil, domain.ErrAlreadyExists
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	c, err := domain.NewCase(r.CaseID, r.SiteCode, r.Coordinates, r.CompletionDigest, r.Surface, r.Actor, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	ev, _ := s.store.AppendEvent(c, "case_created", r.Actor, map[string]any{"site_code": r.SiteCode}, time.Now().Unix())
	resp := map[string]any{"case": c, "event": ev}
	responseBytes, _ := json.Marshal(resp)
	commit := storage.IdempotentResult{Fingerprint: fingerprint(r), Response: responseBytes}
	if err := s.store.Commit(c, ev, r.RequestID, commit); err != nil {
		return nil, err
	}
	return resp, nil
}

type BaselineRequest struct {
	RequestID, CaseID, Actor string
	People                   []string
	ExpectedRevision         int64
	ConfirmedDigest          string
}

type BaselinePrecheckResult struct {
	Issues           []domain.FieldIssue `json:"issues"`
	CanFreeze        bool                `json:"can_freeze"`
	SummaryDigest    string              `json:"summary_digest"`
	ExpectedRevision int64               `json:"expected_revision"`
}

func (s *Service) PrecheckBaseline(caseID, actor string, people []string) (BaselinePrecheckResult, error) {
	c, err := s.store.Get(caseID)
	if err != nil {
		return BaselinePrecheckResult{}, err
	}
	if c.Status != domain.StatusDraft {
		return BaselinePrecheckResult{}, domain.DomainError{Code: "STATE", Message: "只有草拟案件可执行冻结预检"}
	}
	issues, digest := domain.BaselinePrecheck(c, actor, people)
	return BaselinePrecheckResult{Issues: issues, CanFreeze: len(issues) == 0, SummaryDigest: digest, ExpectedRevision: c.Revision}, nil
}

func (s *Service) FreezeBaseline(r BaselineRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "baseline_frozen", func(c *domain.ClosureCase) (string, error) {
		if strings.TrimSpace(r.ConfirmedDigest) == "" {
			return r.Actor, domain.DomainError{Code: "BASELINE_DIGEST", Message: "冻结请求必须携带预检确认摘要"}
		}
		return r.Actor, c.FreezeBaselineConfirmed(r.Actor, r.People, r.ConfirmedDigest, time.Now().UTC())
	})
}

type PlanRequest struct {
	RequestID, CaseID, PreparedBy string
	Layers                        []domain.LayerSpec
	ExpectedRevision              int64
}

func (s *Service) PreparePlan(r PlanRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "plan_prepared", func(c *domain.ClosureCase) (string, error) {
		return r.PreparedBy, c.SetPlan(domain.BackfillPlan{PlanID: fmt.Sprintf("plan-%d", time.Now().UnixNano()), CaseID: r.CaseID, PreparedBy: r.PreparedBy, Layers: r.Layers, PlanDigest: domain.CanonicalDigest(r.Layers)})
	})
}

type ApproveRequest struct {
	RequestID, CaseID, Actor string
	ExpectedRevision         int64
	PlanDigest               string
	ConfirmedRiskCodes       []string
}

func (s *Service) ApprovePlan(r ApproveRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "plan_approved", func(c *domain.ClosureCase) (string, error) {
		if strings.TrimSpace(r.PlanDigest) == "" {
			return r.Actor, domain.DomainError{Code: "PLAN_DIGEST_CONFLICT", Message: "批准请求必须携带当前 plan_digest"}
		}
		return r.Actor, c.ApprovePlanChecked(r.Actor, r.PlanDigest, r.ConfirmedRiskCodes, time.Now().UTC())
	})
}

type LayerRequest struct {
	RequestID, CaseID, ExecutionID string
	LayerIndex                     int
	MaterialCode                   string
	ThicknessMM                    int
	Moisture, Compaction           float64
	PerformedBy, EvidenceDigest    string
	ExpectedRevision               int64
}

func (s *Service) SubmitLayer(r LayerRequest) (any, error) {
	return s.mutateOptions(r.CaseID, r.RequestID, r, r.ExpectedRevision, "layer_submitted", true, func(c *domain.ClosureCase) (string, error) {
		failed, err := c.SubmitLayer(domain.LayerExecution{ExecutionID: r.ExecutionID, CaseID: r.CaseID, LayerIndex: r.LayerIndex, MaterialCode: r.MaterialCode, ActualThicknessMM: r.ThicknessMM, MoisturePercent: r.Moisture, CompactionPercent: r.Compaction, PerformedBy: r.PerformedBy, EvidenceDigest: r.EvidenceDigest}, time.Now().UTC())
		if err != nil {
			return r.PerformedBy, err
		}
		if len(failed) > 0 {
			d := domain.DefectRecord{DefectID: fmt.Sprintf("defect-%d", time.Now().UnixNano()), CaseID: r.CaseID, LayerIndex: r.LayerIndex, FailedRuleCodes: failed, Source: "construction"}
			if e := c.AddDefect(d); e != nil {
				return r.PerformedBy, e
			}
		}
		return r.PerformedBy, nil
	})
}

type DraftRequest struct {
	CaseID                      string
	LayerIndex                  int
	MaterialCode                string
	ThicknessMM                 *int
	Moisture, Compaction        *float64
	PerformedBy, EvidenceDigest string
	ExpectedDraftVersion        int64
}

func (s *Service) SaveLayerDraft(r DraftRequest) (domain.LayerDraft, error) {
	m := s.lock(r.CaseID)
	defer m.Unlock()
	c, err := s.store.Get(r.CaseID)
	if err != nil {
		return domain.LayerDraft{}, err
	}
	if c.Status != domain.StatusPlanReady && c.Status != domain.StatusInProgress {
		return domain.LayerDraft{}, domain.DomainError{Code: "STATE", Message: "当前不允许保存施工草稿"}
	}
	d := domain.LayerDraft{CaseID: r.CaseID, LayerIndex: r.LayerIndex, MaterialCode: strings.TrimSpace(r.MaterialCode), ActualThicknessMM: r.ThicknessMM, MoisturePercent: r.Moisture, CompactionPercent: r.Compaction, PerformedBy: strings.TrimSpace(r.PerformedBy), EvidenceDigest: strings.TrimSpace(r.EvidenceDigest), UpdatedAt: time.Now().UTC()}
	if err := domain.ValidateDraftValues(d); err != nil {
		return domain.LayerDraft{}, err
	}
	if c.Plan == nil || d.LayerIndex != c.NextLayer() {
		return domain.LayerDraft{}, domain.DomainError{Code: "LAYER_ORDER", Message: "只能保存当前应施工层草稿"}
	}
	d.MissingFields = domain.MissingDraftFields(d, c.Plan.Layers[d.LayerIndex-1].EvidenceRequired)
	return s.store.SaveDraft(d, r.ExpectedDraftVersion)
}
func (s *Service) CheckLayerDraft(caseID string) (domain.LayerCheck, error) {
	m := s.lock(caseID)
	defer m.Unlock()
	c, err := s.store.Get(caseID)
	if err != nil {
		return domain.LayerCheck{}, err
	}
	d, ok := s.store.GetDraft(caseID)
	if !ok {
		return domain.LayerCheck{}, domain.DomainError{Code: "DRAFT_NOT_FOUND", Message: "当前层没有施工草稿"}
	}
	return c.CheckDraft(d), nil
}
func (s *Service) SubmitLayerDraft(caseID, requestID, executionID string, expectedRevision, draftVersion int64) (any, error) {
	payload := struct {
		CaseID           string `json:"case_id"`
		ExecutionID      string `json:"execution_id"`
		DraftVersion     int64  `json:"draft_version"`
		ExpectedRevision int64  `json:"expected_revision"`
	}{caseID, executionID, draftVersion, expectedRevision}
	return s.mutateOptions(caseID, requestID, payload, expectedRevision, "layer_submitted", true, func(c *domain.ClosureCase) (string, error) {
		d, ok := s.store.GetDraft(caseID)
		if !ok {
			return "", domain.DomainError{Code: "DRAFT_NOT_FOUND", Message: "当前层没有施工草稿"}
		}
		if d.DraftVersion != draftVersion {
			return d.PerformedBy, domain.DomainError{Code: "DRAFT_CONFLICT", Message: "施工草稿版本已变化"}
		}
		if !c.CheckDraft(d).CanSubmit {
			return d.PerformedBy, domain.DomainError{Code: "DRAFT_INCOMPLETE", Message: "施工草稿核对未通过"}
		}
		failed, err := c.SubmitLayer(domain.LayerExecution{ExecutionID: executionID, CaseID: caseID, LayerIndex: d.LayerIndex, MaterialCode: d.MaterialCode, ActualThicknessMM: *d.ActualThicknessMM, MoisturePercent: *d.MoisturePercent, CompactionPercent: *d.CompactionPercent, PerformedBy: d.PerformedBy, EvidenceDigest: d.EvidenceDigest}, time.Now().UTC())
		if err != nil {
			return d.PerformedBy, err
		}
		if len(failed) > 0 {
			if err := c.AddDefect(domain.DefectRecord{DefectID: fmt.Sprintf("defect-%d", time.Now().UnixNano()), CaseID: caseID, LayerIndex: d.LayerIndex, FailedRuleCodes: failed, Source: "construction"}); err != nil {
				return d.PerformedBy, err
			}
		}
		return d.PerformedBy, nil
	})
}

type DefectRequest struct {
	RequestID, CaseID, DefectID, Actor, Cause, CorrectiveAction, Evidence string
	Values                                                                map[string]float64
	ExpectedRevision                                                      int64
}

type RemediationPlanRequest struct {
	RequestID, CaseID, DefectID, Actor, CauseCategory, Cause, CorrectiveAction, Responsible string
	PlannedCompletionAt                                                                     time.Time
	ExpectedRevision                                                                        int64
}

func (s *Service) PlanRemediation(r RemediationPlanRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "remediation_planned", func(c *domain.ClosureCase) (string, error) {
		return r.Actor, c.RecordRemediationPlan(r.DefectID, r.CauseCategory, r.Cause, r.CorrectiveAction, r.Responsible, r.PlannedCompletionAt, r.Actor, time.Now().UTC())
	})
}

type RemediationCompleteRequest struct {
	RequestID, CaseID, DefectID, Actor, Description, Evidence string
	ExpectedRevision                                          int64
}

func (s *Service) CompleteRemediation(r RemediationCompleteRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "remediation_completed", func(c *domain.ClosureCase) (string, error) {
		return r.Actor, c.CompleteRemediation(r.DefectID, r.Description, r.Evidence, r.Actor, time.Now().UTC())
	})
}

type RetestRequest struct {
	RequestID, CaseID, DefectID, Actor, Evidence string
	Values                                       map[string]float64
	ExpectedRevision                             int64
}

func (s *Service) RetestStaged(r RetestRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "defect_retested", func(c *domain.ClosureCase) (string, error) {
		passed, remaining, err := c.RetestCompletedDefect(r.DefectID, r.Actor, r.Evidence, r.Values, time.Now().UTC())
		if err == nil && !passed {
			_ = remaining
		}
		return r.Actor, err
	})
}

func (s *Service) Retest(r DefectRequest) (any, error) {
	return s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "defect_retested", func(c *domain.ClosureCase) (string, error) {
		if err := domain.ValidateRemediation(r.Cause, r.CorrectiveAction, r.Evidence, r.Actor); err != nil {
			return r.Actor, err
		}
		for i := range c.Defects {
			if c.Defects[i].DefectID == r.DefectID {
				c.Defects[i].Cause = r.Cause
				c.Defects[i].CorrectiveAction = r.CorrectiveAction
			}
		}
		return r.Actor, c.RetestDefect(r.DefectID, r.Actor, r.Evidence, r.Values, time.Now().UTC())
	})
}

type ReviewRequest struct {
	RequestID, CaseID, Actor, Decision string
	ExpectedRevision                   int64
	Issues                             []domain.ReviewIssue
}

func (s *Service) Review(r ReviewRequest) (any, error) {
	out, err := s.mutate(r.CaseID, r.RequestID, r, r.ExpectedRevision, "review_completed", func(c *domain.ClosureCase) (string, error) {
		return r.Actor, c.ReviewWithIssues(r.Actor, r.Decision, r.Issues, time.Now().UTC())
	})
	if err != nil || r.Decision != "pass" {
		return out, err
	}
	c, err := s.store.Get(r.CaseID)
	if err != nil {
		return nil, err
	}
	dossier, err := s.evidence.Build(c, s.store.Events(r.CaseID))
	if err != nil {
		return nil, err
	}
	if err := s.store.SaveDossier(dossier); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) GetCase(id string) (*domain.ClosureCase, error) { return s.store.Get(id) }
func (s *Service) Events(id string) []domain.Event                { return s.store.Events(id) }
func (s *Service) Verify(id string) (evidence.Verification, error) {
	c, err := s.store.Get(id)
	if err != nil {
		return evidence.Verification{}, err
	}
	return s.evidence.Verify(c, s.store.Events(id)), nil
}
func (s *Service) Dossier(id string) (evidence.ClosureDossier, error) {
	dossier, err := s.store.LoadDossier(id)
	if err == nil {
		return dossier, nil
	}
	c, caseErr := s.store.Get(id)
	if caseErr != nil {
		return evidence.ClosureDossier{}, caseErr
	}
	return s.evidence.Build(c, s.store.Events(id))
}
func (s *Service) VerificationReport(id string) (evidence.VerificationReport, error) {
	c, err := s.store.Get(id)
	if err != nil {
		return evidence.VerificationReport{}, err
	}
	if c.Status != domain.StatusSealed {
		return evidence.VerificationReport{}, domain.DomainError{Code: "CASE_NOT_SEALED", Message: "案件尚未封存"}
	}
	d, err := s.store.LoadDossier(id)
	if err != nil {
		if os.IsNotExist(err) {
			return evidence.VerificationReport{}, domain.DomainError{Code: "DOSSIER_NOT_FOUND", Message: "封护档案不存在"}
		}
		return evidence.VerificationReport{}, err
	}
	return s.evidence.Report(c, s.store.Events(id), d, time.Now().UTC()), nil
}
