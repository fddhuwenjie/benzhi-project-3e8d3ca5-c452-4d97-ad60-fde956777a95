package storage

import (
	"fmt"

	"siteclosure/internal/domain"
)

type RecoveryReport struct {
	Cases          int `json:"cases"`
	Events         int `json:"events"`
	SealedCases    int `json:"sealed_cases"`
	IdempotentKeys int `json:"idempotent_keys"`
}

func (s *Store) validateRecoveredState() (RecoveryReport, error) {
	report := RecoveryReport{Cases: len(s.cases), IdempotentKeys: len(s.idem)}
	for caseID, caseState := range s.cases {
		if caseState.CaseID != caseID {
			return report, fmt.Errorf("snapshot key differs from case id %s", caseID)
		}
		if err := domain.ValidateAggregate(caseState); err != nil {
			return report, fmt.Errorf("invalid aggregate %s: %w", caseID, err)
		}
		events := s.events[caseID]
		if len(events) == 0 {
			return report, fmt.Errorf("case %s has no audit events", caseID)
		}
		last := events[len(events)-1]
		if last.Revision != caseState.Revision {
			return report, fmt.Errorf("case %s snapshot revision differs from event history", caseID)
		}
		report.Events += len(events)
		if caseState.Status == domain.StatusSealed {
			report.SealedCases++
		}
	}
	for caseID := range s.events {
		if _, exists := s.cases[caseID]; !exists {
			return report, fmt.Errorf("orphan event history for %s", caseID)
		}
	}
	for requestID, result := range s.idem {
		if requestID == "" {
			return report, fmt.Errorf("empty idempotency key")
		}
		if result.Fingerprint == "" || len(result.Response) == 0 {
			return report, fmt.Errorf("incomplete idempotency result for %s", requestID)
		}
	}
	for caseID, draft := range s.drafts {
		caseState, exists := s.cases[caseID]
		if !exists || draft.CaseID != caseID {
			return report, fmt.Errorf("orphan layer draft for %s", caseID)
		}
		if draft.DraftVersion < 1 {
			return report, fmt.Errorf("invalid layer draft version for %s", caseID)
		}
		if err := domain.ValidateDraftValues(draft); err != nil {
			return report, fmt.Errorf("invalid layer draft for %s: %w", caseID, err)
		}
		if caseState.Status != domain.StatusPlanReady && caseState.Status != domain.StatusInProgress {
			return report, fmt.Errorf("layer draft is not allowed for case %s in status %s", caseID, caseState.Status)
		}
		if draft.LayerIndex != caseState.NextLayer() {
			return report, fmt.Errorf("stale layer draft for %s", caseID)
		}
	}
	return report, nil
}

func (s *Store) RecoveryReport() (RecoveryReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateRecoveredState()
}
