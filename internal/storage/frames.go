package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"siteclosure/internal/domain"
)

const maxEventFrameSize = 4 << 20

func (s *Store) framePath(caseID string) (string, error) {
	if caseID == "" || strings.ContainsAny(caseID, `/\\`) || caseID == "." || caseID == ".." {
		return "", fmt.Errorf("invalid case id for event frames")
	}
	return filepath.Join(s.dir, "events", caseID+".frames"), nil
}

func encodeFrames(events []domain.Event) ([]byte, error) {
	var buffer bytes.Buffer
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("encode event %d: %w", event.Sequence, err)
		}
		if len(payload) == 0 || len(payload) > maxEventFrameSize {
			return nil, fmt.Errorf("event %d has invalid frame size", event.Sequence)
		}
		if err := binary.Write(&buffer, binary.BigEndian, uint32(len(payload))); err != nil {
			return nil, err
		}
		if _, err := buffer.Write(payload); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func decodeFrames(reader io.Reader) ([]domain.Event, error) {
	events := make([]domain.Event, 0)
	for {
		var size uint32
		err := binary.Read(reader, binary.BigEndian, &size)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("truncated event frame header: %w", err)
		}
		if size == 0 || size > maxEventFrameSize {
			return nil, fmt.Errorf("invalid event frame length %d", size)
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("truncated event frame payload: %w", err)
		}
		var event domain.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("decode event frame: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) writeFramesLocked(caseID string) error {
	path, err := s.framePath(caseID)
	if err != nil {
		return err
	}
	data, err := encodeFrames(s.events[caseID])
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0644)
}

func (s *Store) readFrames(caseID string) ([]domain.Event, error) {
	path, err := s.framePath(caseID)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return decodeFrames(file)
}

func (s *Store) verifyFrameFiles() error {
	for caseID, snapshotEvents := range s.events {
		framedEvents, err := s.readFrames(caseID)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("event frame file missing for %s", caseID)
		}
		if err != nil {
			return fmt.Errorf("read event frames for %s: %w", caseID, err)
		}
		if len(framedEvents) != len(snapshotEvents) {
			return fmt.Errorf("event frame count differs for %s", caseID)
		}
		for index := range framedEvents {
			if framedEvents[index].Digest != snapshotEvents[index].Digest {
				return fmt.Errorf("event frame digest differs at %s sequence %d", caseID, index+1)
			}
		}
		if err := domain.ValidateEventChain(caseID, framedEvents); err != nil {
			return fmt.Errorf("invalid framed events for %s: %w", caseID, err)
		}
	}
	return nil
}
