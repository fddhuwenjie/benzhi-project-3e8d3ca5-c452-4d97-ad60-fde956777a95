package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type eventDigestPayload struct {
	Sequence       int64          `json:"sequence"`
	CaseID         string         `json:"case_id"`
	Revision       int64          `json:"revision"`
	Type           string         `json:"type"`
	At             time.Time      `json:"at"`
	Actor          string         `json:"actor"`
	Payload        map[string]any `json:"payload"`
	PreviousDigest string         `json:"previous_digest"`
}

func EventDigest(event Event) string {
	return CanonicalDigest(eventDigestPayload{
		Sequence:       event.Sequence,
		CaseID:         event.CaseID,
		Revision:       event.Revision,
		Type:           event.Type,
		At:             event.At.UTC(),
		Actor:          event.Actor,
		Payload:        event.Payload,
		PreviousDigest: event.PreviousDigest,
	})
}

func ValidateEventChain(caseID string, events []Event) error {
	previous := ""
	var revision int64
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			return fmt.Errorf("案件 %s 的事件序号 %d 不连续", caseID, event.Sequence)
		}
		if event.CaseID != caseID {
			return fmt.Errorf("事件 %d 的案件标识不匹配", event.Sequence)
		}
		if event.Revision < revision {
			return fmt.Errorf("事件 %d 的案件版本发生倒退", event.Sequence)
		}
		if event.PreviousDigest != previous {
			return fmt.Errorf("事件 %d 的前序摘要不匹配", event.Sequence)
		}
		expected := EventDigest(event)
		if event.Digest != expected {
			return fmt.Errorf("事件 %d 的内容摘要校验失败", event.Sequence)
		}
		previous = event.Digest
		revision = event.Revision
	}
	return nil
}

// normalizePayload converts a payload that may hold typed struct values into a
// tree of generic Go values (map[string]any, []any, string, float64, bool, nil)
// via a JSON round-trip. This guarantees that event digests remain stable across
// JSON marshalling and persistence: a value computed from a typed struct and the
// same value reconstructed from a generic map produce identical JSON and thus
// identical digests. It also ensures callers querying events cannot mutate the
// storage's internal typed references through the returned payload.
func normalizePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return payload
	}
	return out
}

func NewEvent(caseID string, revision int64, sequence int64, eventType, actor string, payload map[string]any, at time.Time, previous string) Event {
	event := Event{
		Sequence:       sequence,
		CaseID:         caseID,
		Revision:       revision,
		Type:           eventType,
		At:             at.UTC(),
		Actor:          actor,
		Payload:        normalizePayload(payload),
		PreviousDigest: previous,
	}
	event.Digest = EventDigest(event)
	return event
}
