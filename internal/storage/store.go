package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"siteclosure/internal/domain"
)

type Store struct {
	dir    string
	mu     sync.RWMutex
	cases  map[string]*domain.ClosureCase
	events map[string][]domain.Event
	idem   map[string]IdempotentResult
	drafts map[string]domain.LayerDraft
}
type IdempotentResult struct {
	Fingerprint string          `json:"fingerprint"`
	Response    json.RawMessage `json:"response"`
}
type disk struct {
	Cases  map[string]*domain.ClosureCase `json:"cases"`
	Events map[string][]domain.Event      `json:"events"`
	Idem   map[string]IdempotentResult    `json:"idem"`
	Drafts map[string]domain.LayerDraft   `json:"drafts,omitempty"`
}

func New(dir string) (*Store, error) {
	if dir == "" {
		dir = ".siteclosure-data"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, cases: map[string]*domain.ClosureCase{}, events: map[string][]domain.Event{}, idem: map[string]IdempotentResult{}, drafts: map[string]domain.LayerDraft{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) file() string { return filepath.Join(s.dir, "state.json") }
func (s *Store) load() error {
	b, err := os.ReadFile(s.file())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var d disk
	if err = json.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("state decode: %w", err)
	}
	if d.Cases != nil {
		s.cases = d.Cases
	}
	if d.Events != nil {
		s.events = d.Events
	}
	if d.Idem != nil {
		s.idem = d.Idem
	}
	if d.Drafts != nil {
		s.drafts = d.Drafts
	}
	if err := s.verify(); err != nil {
		return err
	}
	if _, err := s.validateRecoveredState(); err != nil {
		return err
	}
	return s.verifyFrameFiles()
}
func (s *Store) verify() error {
	for id, es := range s.events {
		if err := domain.ValidateEventChain(id, es); err != nil {
			return fmt.Errorf("event chain invalid for %s: %w", id, err)
		}
	}
	return nil
}
func (s *Store) persistLocked() error {
	d := disk{Cases: s.cases, Events: s.events, Idem: s.idem, Drafts: s.drafts}
	return writeAtomicJSON(s.file(), d, 0644)
}

// rollbackCase restores the in-memory case and event history to their
// pre-commit state.  It is used when a transaction must be aborted after
// the case and events were already mutated in memory.
func (s *Store) rollbackCase(caseID string, oldCase *domain.ClosureCase, hadCase bool, oldEvents []domain.Event) {
	if hadCase {
		s.cases[caseID] = oldCase
	} else {
		delete(s.cases, caseID)
	}
	if oldEvents != nil {
		s.events[caseID] = oldEvents
	} else {
		delete(s.events, caseID)
	}
}

// rollbackCommit restores the in-memory state (case, events, idempotency entry
// and draft) to its pre-commit state.  It is used when persistLocked succeeded
// but writeFramesLocked failed, so the snapshot on disk also needs to be
// rewritten by the caller via persistLocked.
func (s *Store) rollbackCommit(caseID, requestID string, oldIdem IdempotentResult, hadIdem bool, oldCase *domain.ClosureCase, hadCase bool, oldEvents []domain.Event, oldDraft domain.LayerDraft, hadDraft, deleteDraft bool) {
	if hadIdem {
		s.idem[requestID] = oldIdem
	} else {
		delete(s.idem, requestID)
	}
	s.rollbackCase(caseID, oldCase, hadCase, oldEvents)
	if deleteDraft {
		if hadDraft {
			s.drafts[caseID] = oldDraft
		} else {
			delete(s.drafts, caseID)
		}
	}
}

func clone(c *domain.ClosureCase) *domain.ClosureCase {
	b, _ := json.Marshal(c)
	var x domain.ClosureCase
	_ = json.Unmarshal(b, &x)
	return &x
}
func (s *Store) Get(id string) (*domain.ClosureCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cases[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return clone(c), nil
}
func (s *Store) Save(c *domain.ClosureCase, event domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.cases[c.CaseID]; ok && old.Status == domain.StatusSealed {
		return domain.ErrSealed
	}
	oldCase, hadCase := s.cases[c.CaseID]
	oldEvents := append([]domain.Event(nil), s.events[c.CaseID]...)
	s.cases[c.CaseID] = clone(c)
	s.events[c.CaseID] = append(s.events[c.CaseID], event)
	if err := s.persistLocked(); err != nil {
		s.rollbackCase(c.CaseID, oldCase, hadCase, oldEvents)
		return err
	}
	if err := s.writeFramesLocked(c.CaseID); err != nil {
		s.rollbackCase(c.CaseID, oldCase, hadCase, oldEvents)
		_ = s.persistLocked()
		return err
	}
	return nil
}

func (s *Store) Commit(c *domain.ClosureCase, event domain.Event, requestID string, result IdempotentResult) error {
	return s.commit(c, event, requestID, result, false)
}

func (s *Store) CommitAndDeleteDraft(c *domain.ClosureCase, event domain.Event, requestID string, result IdempotentResult) error {
	return s.commit(c, event, requestID, result, true)
}

func (s *Store) commit(c *domain.ClosureCase, event domain.Event, requestID string, result IdempotentResult, deleteDraft bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.cases[c.CaseID]; ok && old.Status == domain.StatusSealed {
		return domain.ErrSealed
	}
	if existing, ok := s.idem[requestID]; ok {
		if existing.Fingerprint != result.Fingerprint {
			return fmt.Errorf("idempotency fingerprint conflict")
		}
		return nil
	}
	oldCase, hadCase := s.cases[c.CaseID]
	oldEvents := append([]domain.Event(nil), s.events[c.CaseID]...)
	oldDraft, hadDraft := s.drafts[c.CaseID]
	oldIdem, hadIdem := s.idem[requestID]
	s.cases[c.CaseID] = clone(c)
	s.events[c.CaseID] = append(s.events[c.CaseID], event)
	s.idem[requestID] = result
	if deleteDraft {
		delete(s.drafts, c.CaseID)
	}
	if err := s.persistLocked(); err != nil {
		s.rollbackCommit(c.CaseID, requestID, oldIdem, hadIdem, oldCase, hadCase, oldEvents, oldDraft, hadDraft, deleteDraft)
		return err
	}
	if err := s.writeFramesLocked(c.CaseID); err != nil {
		s.rollbackCommit(c.CaseID, requestID, oldIdem, hadIdem, oldCase, hadCase, oldEvents, oldDraft, hadDraft, deleteDraft)
		_ = s.persistLocked()
		return err
	}
	return nil
}

func (s *Store) GetDraft(caseID string) (domain.LayerDraft, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	draft, ok := s.drafts[caseID]
	return draft, ok
}

func (s *Store) SaveDraft(draft domain.LayerDraft, expectedVersion int64) (domain.LayerDraft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.drafts[draft.CaseID]
	if expectedVersion != 0 {
		if !exists || current.DraftVersion != expectedVersion {
			return domain.LayerDraft{}, domain.DomainError{Code: "DRAFT_CONFLICT", Message: "施工草稿已被其他页面更新"}
		}
	} else if exists {
		return domain.LayerDraft{}, domain.DomainError{Code: "DRAFT_CONFLICT", Message: "施工草稿已存在，请先加载最新版本"}
	}
	draft.DraftVersion = current.DraftVersion + 1
	s.drafts[draft.CaseID] = draft
	if err := s.persistLocked(); err != nil {
		if exists {
			s.drafts[draft.CaseID] = current
		} else {
			delete(s.drafts, draft.CaseID)
		}
		return domain.LayerDraft{}, err
	}
	return draft, nil
}

type Snapshot struct {
	Cases  map[string]*domain.ClosureCase
	Events map[string][]domain.Event
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Snapshot{Cases: make(map[string]*domain.ClosureCase, len(s.cases)), Events: make(map[string][]domain.Event, len(s.events))}
	for id, c := range s.cases {
		out.Cases[id] = clone(c)
		out.Events[id] = append([]domain.Event(nil), s.events[id]...)
	}
	return out
}
func (s *Store) Events(id string) []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Event(nil), s.events[id]...)
}
func (s *Store) GetIdem(id string) (IdempotentResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.idem[id]
	return r, ok
}
func (s *Store) PutIdem(id string, r IdempotentResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idem[id] = r
	return s.persistLocked()
}
func (s *Store) Dir() string { return s.dir }
