package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"siteclosure/internal/evidence"
)

func (s *Store) dossierPath(caseID string) (string, error) {
	if caseID == "" || strings.ContainsAny(caseID, `/\\`) || caseID == "." || caseID == ".." {
		return "", fmt.Errorf("invalid case id for dossier")
	}
	return filepath.Join(s.dir, "dossiers", caseID+".json"), nil
}

func (s *Store) SaveDossier(dossier evidence.ClosureDossier) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDossierLocked(dossier)
}

// saveDossierLocked persists the sealed dossier. The caller must hold s.mu.
// Duplicates with an identical digest are tolerated so that replaying a
// successfully sealed review remains idempotent.
func (s *Store) saveDossierLocked(dossier evidence.ClosureDossier) error {
	path, err := s.dossierPath(dossier.CaseID)
	if err != nil {
		return err
	}
	if existing, loadErr := s.loadDossierLocked(dossier.CaseID); loadErr == nil {
		if existing.SHA256Digest != dossier.SHA256Digest {
			return fmt.Errorf("sealed dossier already exists with a different digest")
		}
		return nil
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	if err := writeAtomicJSON(path, dossier, 0444); err != nil {
		return fmt.Errorf("persist dossier: %w", err)
	}
	return nil
}

func (s *Store) LoadDossier(caseID string) (evidence.ClosureDossier, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadDossierLocked(caseID)
}

// loadDossierLocked reads and validates the stored dossier. The caller must
// hold s.mu (at least for reading).
func (s *Store) loadDossierLocked(caseID string) (evidence.ClosureDossier, error) {
	path, err := s.dossierPath(caseID)
	if err != nil {
		return evidence.ClosureDossier{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return evidence.ClosureDossier{}, err
	}
	var dossier evidence.ClosureDossier
	if err := json.Unmarshal(data, &dossier); err != nil {
		return evidence.ClosureDossier{}, fmt.Errorf("decode dossier: %w", err)
	}
	if dossier.CaseID != caseID {
		return evidence.ClosureDossier{}, fmt.Errorf("dossier case id mismatch")
	}
	if dossier.SHA256Digest == "" || len(dossier.CanonicalPayload) == 0 {
		return evidence.ClosureDossier{}, fmt.Errorf("dossier is incomplete")
	}
	return dossier, nil
}
