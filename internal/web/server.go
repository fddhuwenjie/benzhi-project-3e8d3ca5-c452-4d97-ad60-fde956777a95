package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"siteclosure/internal/application"
	"siteclosure/internal/domain"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }
func (s *Server) routes() {
	sub, _ := fs.Sub(assets, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	s.mux.HandleFunc("GET /", s.HandleIndex)
	s.mux.HandleFunc("POST /api/cases", s.HandleCreateCase)
	s.mux.HandleFunc("GET /api/cases", s.HandleListCases)
	s.mux.HandleFunc("GET /api/cases/{caseID}", s.HandleGetCase)
	s.mux.HandleFunc("POST /api/cases/{caseID}/baseline", s.HandleFreezeBaseline)
	s.mux.HandleFunc("POST /api/cases/{caseID}/baseline/precheck", s.HandleBaselinePrecheck)
	s.mux.HandleFunc("POST /api/cases/{caseID}/plan", s.HandlePreparePlan)
	s.mux.HandleFunc("POST /api/cases/{caseID}/approve", s.HandleApprovePlan)
	s.mux.HandleFunc("POST /api/cases/{caseID}/layers", s.HandleSubmitLayer)
	s.mux.HandleFunc("PUT /api/cases/{caseID}/layer-draft", s.HandleSaveLayerDraft)
	s.mux.HandleFunc("GET /api/cases/{caseID}/layer-draft/check", s.HandleCheckLayerDraft)
	s.mux.HandleFunc("POST /api/cases/{caseID}/layer-draft/submit", s.HandleSubmitLayerDraft)
	s.mux.HandleFunc("POST /api/cases/{caseID}/defects/{defectID}/plan", s.HandleRemediationPlan)
	s.mux.HandleFunc("POST /api/cases/{caseID}/defects/{defectID}/complete", s.HandleRemediationComplete)
	s.mux.HandleFunc("POST /api/cases/{caseID}/defects/{defectID}/retest", s.HandleRetest)
	s.mux.HandleFunc("POST /api/cases/{caseID}/review", s.HandleReview)
	s.mux.HandleFunc("GET /api/cases/{caseID}/events", s.HandleEvents)
	s.mux.HandleFunc("GET /api/cases/{caseID}/verify", s.HandleVerify)
	s.mux.HandleFunc("GET /api/cases/{caseID}/verification-report", s.HandleVerificationReport)
	s.mux.HandleFunc("GET /api/cases/{caseID}/dossier", s.HandleDossier)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := assets.ReadFile("static/index.html")
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeJSON(w, 400, map[string]any{"error": "INVALID_JSON", "message": "请求格式不正确: " + err.Error()})
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	status := 422
	code := "BUSINESS_RULE"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = 404
		code = "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		status = 409
		code = "REVISION_CONFLICT"
	case errors.Is(err, domain.ErrAlreadyExists):
		status = 409
		code = "CASE_ALREADY_EXISTS"
	case errors.Is(err, domain.ErrSealed):
		status = 409
		code = "CASE_SEALED"
	}
	var de domain.DomainError
	if errors.As(err, &de) {
		code = de.Code
		switch de.Code {
		case "IDEMPOTENCY", "PLAN_DIGEST_CONFLICT", "DRAFT_CONFLICT", "BASELINE_DIGEST":
			status = http.StatusConflict
		}
	}
	writeJSON(w, status, map[string]any{"error": code, "message": err.Error()})
}
func requestID(r *http.Request, body string) string {
	if x := strings.TrimSpace(r.Header.Get("Idempotency-Key")); x != "" {
		return x
	}
	return body
}
