package web

import (
	"net/http"
	"siteclosure/internal/application"
	"siteclosure/internal/domain"
	"time"
)

func (s *Server) HandleCreateCase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID               string `json:"request_id"`
		CaseID                  string `json:"case_id"`
		SiteCode                string `json:"site_code"`
		TrenchCoordinates       string `json:"trench_coordinates"`
		CompletionRecordDigest  string `json:"completion_record_digest"`
		ExposedSurfaceCondition string `json:"exposed_surface_condition"`
		CreatedBy               string `json:"created_by"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.CreateCase(application.CreateCaseRequest{RequestID: requestID(r, in.RequestID), CaseID: in.CaseID, SiteCode: in.SiteCode, Coordinates: in.TrenchCoordinates, CompletionDigest: in.CompletionRecordDigest, Surface: in.ExposedSurfaceCondition, Actor: in.CreatedBy})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (s *Server) HandleFreezeBaseline(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string   `json:"request_id"`
		Actor            string   `json:"actor"`
		People           []string `json:"people"`
		ExpectedRevision int64    `json:"expected_revision"`
		ConfirmedDigest  string   `json:"confirmed_digest"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.FreezeBaseline(application.BaselineRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), Actor: in.Actor, People: in.People, ExpectedRevision: in.ExpectedRevision, ConfirmedDigest: in.ConfirmedDigest})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleBaselinePrecheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Actor  string   `json:"actor"`
		People []string `json:"people"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.PrecheckBaseline(r.PathValue("caseID"), in.Actor, in.People)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandlePreparePlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string             `json:"request_id"`
		PreparedBy       string             `json:"prepared_by"`
		ExpectedRevision int64              `json:"expected_revision"`
		Layers           []domain.LayerSpec `json:"layers"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.PreparePlan(application.PlanRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), PreparedBy: in.PreparedBy, Layers: in.Layers, ExpectedRevision: in.ExpectedRevision})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleApprovePlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID          string   `json:"request_id"`
		Actor              string   `json:"actor"`
		ExpectedRevision   int64    `json:"expected_revision"`
		PlanDigest         string   `json:"plan_digest"`
		ConfirmedRiskCodes []string `json:"confirmed_risk_codes"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.ApprovePlan(application.ApproveRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), Actor: in.Actor, ExpectedRevision: in.ExpectedRevision, PlanDigest: in.PlanDigest, ConfirmedRiskCodes: in.ConfirmedRiskCodes})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleSaveLayerDraft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LayerIndex           int      `json:"layer_index"`
		MaterialCode         string   `json:"material_code"`
		ActualThicknessMM    *int     `json:"actual_thickness_mm"`
		MoisturePercent      *float64 `json:"moisture_percent"`
		CompactionPercent    *float64 `json:"compaction_percent"`
		PerformedBy          string   `json:"performed_by"`
		EvidenceDigest       string   `json:"evidence_digest"`
		ExpectedDraftVersion int64    `json:"expected_draft_version"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.SaveLayerDraft(application.DraftRequest{CaseID: r.PathValue("caseID"), LayerIndex: in.LayerIndex, MaterialCode: in.MaterialCode, ThicknessMM: in.ActualThicknessMM, Moisture: in.MoisturePercent, Compaction: in.CompactionPercent, PerformedBy: in.PerformedBy, EvidenceDigest: in.EvidenceDigest, ExpectedDraftVersion: in.ExpectedDraftVersion})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleSubmitLayerDraft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string `json:"request_id"`
		ExecutionID      string `json:"execution_id"`
		ExpectedRevision int64  `json:"expected_revision"`
		DraftVersion     int64  `json:"draft_version"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.SubmitLayerDraft(r.PathValue("caseID"), requestID(r, in.RequestID), in.ExecutionID, in.ExpectedRevision, in.DraftVersion)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleSubmitLayer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID         string  `json:"request_id"`
		ExecutionID       string  `json:"execution_id"`
		LayerIndex        int     `json:"layer_index"`
		MaterialCode      string  `json:"material_code"`
		ActualThicknessMM int     `json:"actual_thickness_mm"`
		MoisturePercent   float64 `json:"moisture_percent"`
		CompactionPercent float64 `json:"compaction_percent"`
		PerformedBy       string  `json:"performed_by"`
		EvidenceDigest    string  `json:"evidence_digest"`
		ExpectedRevision  int64   `json:"expected_revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.SubmitLayer(application.LayerRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), ExecutionID: in.ExecutionID, LayerIndex: in.LayerIndex, MaterialCode: in.MaterialCode, ThicknessMM: in.ActualThicknessMM, Moisture: in.MoisturePercent, Compaction: in.CompactionPercent, PerformedBy: in.PerformedBy, EvidenceDigest: in.EvidenceDigest, ExpectedRevision: in.ExpectedRevision})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleRetest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string             `json:"request_id"`
		Actor            string             `json:"actor"`
		EvidenceDigest   string             `json:"evidence_digest"`
		RetestValues     map[string]float64 `json:"retest_values"`
		ExpectedRevision int64              `json:"expected_revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.RetestStaged(application.RetestRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), DefectID: r.PathValue("defectID"), Actor: in.Actor, Evidence: in.EvidenceDigest, Values: in.RetestValues, ExpectedRevision: in.ExpectedRevision})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleRemediationPlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID           string    `json:"request_id"`
		Actor               string    `json:"actor"`
		CauseCategory       string    `json:"cause_category"`
		Cause               string    `json:"cause"`
		CorrectiveAction    string    `json:"corrective_action"`
		Responsible         string    `json:"responsible"`
		PlannedCompletionAt time.Time `json:"planned_completion_at"`
		ExpectedRevision    int64     `json:"expected_revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.PlanRemediation(application.RemediationPlanRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), DefectID: r.PathValue("defectID"), Actor: in.Actor, CauseCategory: in.CauseCategory, Cause: in.Cause, CorrectiveAction: in.CorrectiveAction, Responsible: in.Responsible, PlannedCompletionAt: in.PlannedCompletionAt, ExpectedRevision: in.ExpectedRevision})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleRemediationComplete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string `json:"request_id"`
		Actor            string `json:"actor"`
		Description      string `json:"description"`
		EvidenceDigest   string `json:"evidence_digest"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.CompleteRemediation(application.RemediationCompleteRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), DefectID: r.PathValue("defectID"), Actor: in.Actor, Description: in.Description, Evidence: in.EvidenceDigest, ExpectedRevision: in.ExpectedRevision})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID        string               `json:"request_id"`
		Actor            string               `json:"actor"`
		Decision         string               `json:"decision"`
		ExpectedRevision int64                `json:"expected_revision"`
		Issues           []domain.ReviewIssue `json:"issues"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.app.Review(application.ReviewRequest{RequestID: requestID(r, in.RequestID), CaseID: r.PathValue("caseID"), Actor: in.Actor, Decision: in.Decision, ExpectedRevision: in.ExpectedRevision, Issues: in.Issues})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
