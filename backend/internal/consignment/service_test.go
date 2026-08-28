package consignment

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/nswclient"
)

type stubNSWClient struct {
	fetchCount int
	info       *nswclient.ConsignmentAgency
	err        error
}

func (s *stubNSWClient) GetConsignmentAgency(_ context.Context, _ string) (*nswclient.ConsignmentAgency, error) {
	s.fetchCount++
	if s.err != nil {
		return nil, s.err
	}
	return s.info, nil
}

func TestGetConsignments_ReturnsLocalStoreData(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ADAM PVT LTD"}, "t1")

	got, err := NewService(store, &stubNSWClient{}).GetConsignments(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("GetConsignments: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ConsignmentID != "wf1" || got.Items[0].TaskCount != 1 {
		t.Fatalf("unexpected list: %+v", got.Items)
	}
	if got.Items[0].TraderCompanyName != "ADAM PVT LTD" {
		t.Errorf("expected trader company name on list: %+v", got.Items[0])
	}
}

func TestGetConsignments_WithoutExtras_KeepsMainFields(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "wf-plain", "PENDING", "t1")

	got, err := NewService(store, &stubNSWClient{}).GetConsignments(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("GetConsignments: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 consignment, got %+v", got.Items)
	}
	item := got.Items[0]
	if item.ConsignmentID != "wf-plain" || item.Status != "PENDING" || item.TaskCount != 1 {
		t.Errorf("main list fields changed: %+v", item)
	}
	if item.TraderCompanyName != "" {
		t.Errorf("trader company name must stay empty without extras, got %q", item.TraderCompanyName)
	}
}

func TestCreateConsignment_FetchesNSWWhenNew(t *testing.T) {
	store := newTestStore(t)
	nsw := &stubNSWClient{
		info: &nswclient.ConsignmentAgency{ConsignmentID: "c-new", TraderCompanyName: "CEYLON EXPORTS"},
	}
	svc := NewService(store, nsw)
	ctx := context.Background()

	if err := svc.CreateConsignment(ctx, "c-new"); err != nil {
		t.Fatalf("CreateConsignment: %v", err)
	}
	if nsw.fetchCount != 1 {
		t.Errorf("expected 1 NSW fetch, got %d", nsw.fetchCount)
	}

	got, err := svc.GetConsignment(ctx, "c-new")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.TraderCompany != "CEYLON EXPORTS" {
		t.Errorf("expected CEYLON EXPORTS, got %q", got.TraderCompany)
	}

	if err := svc.CreateConsignment(ctx, "c-new"); err != nil {
		t.Fatalf("second CreateConsignment: %v", err)
	}
	if nsw.fetchCount != 1 {
		t.Errorf("cached extras must skip NSW, got fetchCount %d", nsw.fetchCount)
	}
}

func TestCreateConsignment_RetriesWhenExtrasMissing(t *testing.T) {
	store := newTestStore(t)
	nsw := &stubNSWClient{err: errors.New("nsw timeout")}
	svc := NewService(store, nsw)
	ctx := context.Background()

	if err := svc.CreateConsignment(ctx, "c-retry"); err == nil {
		t.Fatal("expected NSW fetch error")
	}

	nsw.err = nil
	nsw.info = &nswclient.ConsignmentAgency{ConsignmentID: "c-retry", TraderCompanyName: "ADAM PVT LTD"}
	if err := svc.CreateConsignment(ctx, "c-retry"); err != nil {
		t.Fatalf("retry CreateConsignment: %v", err)
	}
	if nsw.fetchCount != 2 {
		t.Errorf("expected 2 fetches, got %d", nsw.fetchCount)
	}

	got, err := svc.GetConsignment(ctx, "c-retry")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.TraderCompany != "ADAM PVT LTD" {
		t.Errorf("expected extras on retry, got %q", got.TraderCompany)
	}
}

func TestUpdateConsignment(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "c-upd", "PENDING")
	svc := NewService(store, &stubNSWClient{})

	if err := svc.UpdateConsignment(context.Background(), "c-upd", "APPROVED"); err != nil {
		t.Fatalf("UpdateConsignment: %v", err)
	}
	got, err := svc.GetConsignment(context.Background(), "c-upd")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.Status != "APPROVED" {
		t.Errorf("status: got %q", got.Status)
	}
}
