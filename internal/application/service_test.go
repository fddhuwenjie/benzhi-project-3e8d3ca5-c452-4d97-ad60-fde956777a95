package application

import (
	"errors"
	"testing"

	"siteclosure/internal/domain"
	"siteclosure/internal/storage"
)

func newService(t *testing.T) *Service {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(store)
}
func TestIdempotencyAndRevisionConflict(t *testing.T) {
	s := newService(t)
	create := CreateCaseRequest{RequestID: "request-create", CaseID: "case-a", SiteCode: "SITE", Coordinates: "N/E", CompletionDigest: "digest", Surface: "stable", Actor: "creator"}
	first, err := s.CreateCase(create)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateCase(create)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || second == nil {
		t.Fatal("幂等响应为空")
	}
	_, err = s.CreateCase(CreateCaseRequest{RequestID: "request-create", CaseID: "other", SiteCode: "SITE", Coordinates: "N/E", CompletionDigest: "digest", Surface: "stable", Actor: "creator"})
	if err == nil {
		t.Fatal("同 request_id 异载荷应失败")
	}
	_, err = s.FreezeBaseline(BaselineRequest{RequestID: "freeze", CaseID: "case-a", Actor: "creator", People: []string{"creator"}, ExpectedRevision: 99})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	c, _ := s.GetCase("case-a")
	if c.Revision != 1 {
		t.Fatalf("冲突请求修改了版本: %d", c.Revision)
	}
}
func TestDuplicateCaseIDRejected(t *testing.T) {
	s := newService(t)
	base := CreateCaseRequest{RequestID: "one", CaseID: "same", SiteCode: "SITE", Coordinates: "N/E", CompletionDigest: "digest", Surface: "stable", Actor: "creator"}
	if _, err := s.CreateCase(base); err != nil {
		t.Fatal(err)
	}
	base.RequestID = "two"
	if _, err := s.CreateCase(base); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("got %v", err)
	}
}
