package application

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"siteclosure/internal/domain"
)

type CaseSummary struct {
	CaseID           string            `json:"case_id"`
	SiteCode         string            `json:"site_code"`
	Coordinates      string            `json:"trench_coordinates"`
	Status           domain.CaseStatus `json:"status"`
	Revision         int64             `json:"revision"`
	PlannedLayers    int               `json:"planned_layers"`
	PassedLayers     int               `json:"passed_layers"`
	OpenDefects      int               `json:"open_defects"`
	AllowedActions   []domain.Action   `json:"allowed_actions"`
	LastEvent        *domain.Event     `json:"last_event,omitempty"`
	LastEventSummary string            `json:"last_event_summary,omitempty"`
}

type LayerView struct {
	Index         int                    `json:"index"`
	MaterialCode  string                 `json:"material_code"`
	TargetMM      int                    `json:"target_thickness_mm"`
	ToleranceMM   int                    `json:"thickness_tolerance_mm"`
	MoistureMin   float64                `json:"moisture_min_percent"`
	MoistureMax   float64                `json:"moisture_max_percent"`
	CompactionMin float64                `json:"compaction_min_percent"`
	Execution     *domain.LayerExecution `json:"execution,omitempty"`
	OpenDefectIDs []string               `json:"open_defect_ids"`
}

type CaseView struct {
	Case      *domain.ClosureCase    `json:"case"`
	Summary   CaseSummary            `json:"summary"`
	Profile   []LayerView            `json:"profile"`
	Timeline  []domain.Event         `json:"timeline"`
	Roles     map[string][]string    `json:"participant_roles"`
	Draft     *domain.LayerDraft     `json:"layer_draft,omitempty"`
	Integrity domain.IntegrityStatus `json:"baseline_integrity"`
}

func (s *Service) Summary(id string) (CaseSummary, error) {
	caseState, err := s.store.Get(id)
	if err != nil {
		return CaseSummary{}, err
	}
	events := s.store.Events(id)
	summary := CaseSummary{CaseID: caseState.CaseID, SiteCode: caseState.SiteCode, Coordinates: caseState.TrenchCoordinates, Status: caseState.Status, Revision: caseState.Revision, PassedLayers: caseState.CompletedLayers(), AllowedActions: caseState.AllowedActions()}
	if caseState.Plan != nil {
		summary.PlannedLayers = len(caseState.Plan.Layers)
	}
	for _, defect := range caseState.Defects {
		if defect.Status == domain.DefectOpen {
			summary.OpenDefects++
		}
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		summary.LastEvent = &last
		summary.LastEventSummary = fmt.Sprintf("%s · %s", last.Type, last.Actor)
	}
	return summary, nil
}

func (s *Service) View(id string) (CaseView, error) {
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	caseState, err := s.store.Get(id)
	if err != nil {
		return CaseView{}, err
	}
	summary, err := s.Summary(id)
	if err != nil {
		return CaseView{}, err
	}
	profile := make([]LayerView, 0)
	if caseState.Plan != nil {
		for _, spec := range caseState.Plan.Layers {
			layer := LayerView{Index: spec.Index, MaterialCode: spec.MaterialCode, TargetMM: spec.TargetThicknessMM, ToleranceMM: spec.ThicknessToleranceMM, MoistureMin: spec.MoistureMinPercent, MoistureMax: spec.MoistureMaxPercent, CompactionMin: spec.CompactionMinPercent}
			for executionIndex := range caseState.Executions {
				if caseState.Executions[executionIndex].LayerIndex == spec.Index {
					execution := caseState.Executions[executionIndex]
					layer.Execution = &execution
				}
			}
			for _, defect := range caseState.Defects {
				if defect.LayerIndex == spec.Index && defect.Status == domain.DefectOpen {
					layer.OpenDefectIDs = append(layer.OpenDefectIDs, defect.DefectID)
				}
			}
			sort.Strings(layer.OpenDefectIDs)
			profile = append(profile, layer)
		}
	}
	view := CaseView{Case: caseState, Summary: summary, Profile: profile, Timeline: s.store.Events(id), Roles: caseState.ParticipantRoles(), Integrity: caseState.BaselineIntegrity()}
	if draft, ok := s.store.GetDraft(id); ok {
		view.Draft = &draft
	}
	return view, nil
}

type CaseFilter struct {
	SiteCode       string
	Status         domain.CaseStatus
	Responsible    string
	HasOpenDefect  *bool
	Page, PageSize int
}
type CaseStats struct {
	ByStatus       map[domain.CaseStatus]int `json:"by_status"`
	OpenDefects    int                       `json:"open_defects"`
	AwaitingReview int                       `json:"awaiting_review"`
	Total          int                       `json:"total"`
}
type CaseList struct {
	Items    []CaseSummary `json:"items"`
	Stats    CaseStats     `json:"stats"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int           `json:"total"`
}

func cloneCaseList(list CaseList) CaseList {
	data, _ := json.Marshal(list)
	var cloned CaseList
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func (s *Service) ListCases(filter CaseFilter) (CaseList, error) {
	for name, value := range map[string]string{"site_code": filter.SiteCode, "responsible": filter.Responsible} {
		if utf8.RuneCountInString(value) > 120 {
			return CaseList{}, domain.DomainError{Code: "FILTER_TOO_LONG", Message: name + "筛选值不能超过120个字符"}
		}
	}
	valid := map[domain.CaseStatus]bool{domain.StatusDraft: true, domain.StatusFrozen: true, domain.StatusPlanReady: true, domain.StatusInProgress: true, domain.StatusRemediation: true, domain.StatusAwaitingReview: true, domain.StatusSealed: true}
	if filter.Status != "" && !valid[filter.Status] {
		return CaseList{}, domain.DomainError{Code: "INVALID_STATUS", Message: "未知案件状态"}
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return CaseList{}, domain.DomainError{Code: "INVALID_PAGINATION", Message: "page必须大于0，page_size必须在1到100之间"}
	}
	cacheKey := fingerprint(filter)
	s.caseListCacheMu.RLock()
	cached, ok := s.caseListCache[cacheKey]
	s.caseListCacheMu.RUnlock()
	if ok {
		return cloneCaseList(cached), nil
	}
	snapshot := s.store.Snapshot()
	all := make([]CaseSummary, 0)
	stats := CaseStats{ByStatus: map[domain.CaseStatus]int{}}
	for id, c := range snapshot.Cases {
		if filter.SiteCode != "" && c.SiteCode != strings.TrimSpace(filter.SiteCode) {
			continue
		}
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		if filter.Responsible != "" && !caseHasPerson(c, strings.TrimSpace(filter.Responsible)) {
			continue
		}
		open := len(c.OpenDefects())
		if filter.HasOpenDefect != nil && (*filter.HasOpenDefect) != (open > 0) {
			continue
		}
		events := snapshot.Events[id]
		item := CaseSummary{CaseID: c.CaseID, SiteCode: c.SiteCode, Coordinates: c.TrenchCoordinates, Status: c.Status, Revision: c.Revision, PassedLayers: c.CompletedLayers(), OpenDefects: open, AllowedActions: c.AllowedActions()}
		if c.Plan != nil {
			item.PlannedLayers = len(c.Plan.Layers)
		}
		if len(events) > 0 {
			last := events[len(events)-1]
			item.LastEvent = &last
			item.LastEventSummary = fmt.Sprintf("%s · %s", last.Type, last.Actor)
		}
		all = append(all, item)
		stats.Total++
		stats.ByStatus[c.Status]++
		stats.OpenDefects += open
		if c.Status == domain.StatusAwaitingReview {
			stats.AwaitingReview++
		}
	}
	sort.Slice(all, func(i, j int) bool {
		var ti, tj int64
		if all[i].LastEvent != nil {
			ti = all[i].LastEvent.At.UnixNano()
		}
		if all[j].LastEvent != nil {
			tj = all[j].LastEvent.At.UnixNano()
		}
		if ti == tj {
			return all[i].CaseID < all[j].CaseID
		}
		return ti > tj
	})
	start := (filter.Page - 1) * filter.PageSize
	if start > len(all) {
		start = len(all)
	}
	end := start + filter.PageSize
	if end > len(all) {
		end = len(all)
	}
	result := CaseList{Items: all[start:end], Stats: stats, Page: filter.Page, PageSize: filter.PageSize, Total: len(all)}
	s.caseListCacheMu.Lock()
	s.caseListCache[cacheKey] = cloneCaseList(result)
	s.caseListCacheMu.Unlock()
	return result, nil
}
func caseHasPerson(c *domain.ClosureCase, person string) bool {
	if c.CreatedBy == person {
		return true
	}
	if c.Baseline != nil {
		for _, p := range c.Baseline.ResponsiblePeople {
			if p == person {
				return true
			}
		}
	}
	_, ok := c.ParticipantRoles()[person]
	return ok
}
