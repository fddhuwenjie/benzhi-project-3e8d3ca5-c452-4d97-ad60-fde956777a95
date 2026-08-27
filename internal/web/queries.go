package web

import (
	"net/http"
	"siteclosure/internal/application"
	"siteclosure/internal/domain"
	"strconv"
)

func (s *Server) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	c, err := s.app.View(r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, c)
}
func (s *Server) HandleListCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := strconv.Atoi(q.Get("page"))
	if q.Get("page") == "" {
		page = 1
	} else if err != nil {
		writeError(w, domain.DomainError{Code: "INVALID_PAGINATION", Message: "page必须为整数"})
		return
	}
	size, err := strconv.Atoi(q.Get("page_size"))
	if q.Get("page_size") == "" {
		size = 20
	} else if err != nil {
		writeError(w, domain.DomainError{Code: "INVALID_PAGINATION", Message: "page_size必须为整数"})
		return
	}
	var open *bool
	if raw := q.Get("has_open_defect"); raw != "" {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			writeError(w, domain.DomainError{Code: "INVALID_FILTER", Message: "has_open_defect必须为true或false"})
			return
		}
		open = &v
	}
	out, e := s.app.ListCases(application.CaseFilter{SiteCode: q.Get("site_code"), Status: domain.CaseStatus(q.Get("status")), Responsible: q.Get("responsible"), HasOpenDefect: open, Page: page, PageSize: size})
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleCheckLayerDraft(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.CheckLayerDraft(r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) HandleEvents(w http.ResponseWriter, r *http.Request) {
	if _, err := s.app.GetCase(r.PathValue("caseID")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"events": s.app.Events(r.PathValue("caseID"))})
}
func (s *Server) HandleVerify(w http.ResponseWriter, r *http.Request) {
	v, err := s.app.Verify(r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) HandleDossier(w http.ResponseWriter, r *http.Request) {
	d, err := s.app.Dossier(r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=closure-dossier.json")
	writeJSON(w, 200, d)
}
func (s *Server) HandleVerificationReport(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.VerificationReport(r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", "attachment; filename=closure-verification-report.json")
	}
	writeJSON(w, 200, out)
}
