package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"siteclosure/internal/domain"
)

func TestRecoveryDetectsTamperedFrames(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewCase("case-frames", "SITE", "N/E", "digest", "stable", "actor", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	event, _ := store.AppendEvent(c, "case_created", "actor", map[string]any{"request_id": "one"}, time.Now().Unix())
	if err := store.Save(c, event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events", "case-frames.frames")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("篡改事件帧未被发现")
	}
}
func TestFrameDecoderRejectsTruncation(t *testing.T) {
	event := domain.NewEvent("c", 1, 1, "created", "actor", map[string]any{"x": "y"}, time.Now().UTC(), "")
	data, err := encodeFrames([]domain.Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFrames(bytes.NewReader(data[:len(data)-2])); err == nil {
		t.Fatal("截断帧未被拒绝")
	}
}
