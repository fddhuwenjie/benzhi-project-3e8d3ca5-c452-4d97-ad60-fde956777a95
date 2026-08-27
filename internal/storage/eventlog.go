package storage

import (
	"encoding/json"
	"fmt"
	"siteclosure/internal/domain"
	"time"
)

func (s *Store) AppendEvent(c *domain.ClosureCase, typ, actor string, payload map[string]any, nowUnix int64) (domain.Event, error) {
	es := s.Events(c.CaseID)
	prev := ""
	if len(es) > 0 {
		prev = es[len(es)-1].Digest
	}
	e := domain.NewEvent(c.CaseID, c.Revision, int64(len(es)+1), typ, actor, payload, time.Unix(nowUnix, 0), prev)
	return e, nil
}

func EncodeEvent(e domain.Event) []byte { b, _ := json.Marshal(e); return b }
func VerifyEvents(es []domain.Event) error {
	if len(es) == 0 {
		return nil
	}
	if err := domain.ValidateEventChain(es[0].CaseID, es); err != nil {
		return fmt.Errorf("event chain invalid: %w", err)
	}
	return nil
}
