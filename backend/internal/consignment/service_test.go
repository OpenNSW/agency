package consignment

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/nswclient"
)

type fakeNSW struct {
	byID map[string]*nswclient.ConsignmentAgency
	err  error
}

func (f *fakeNSW) GetConsignmentAgency(_ context.Context, consignmentID string) (*nswclient.ConsignmentAgency, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.byID == nil {
		return nil, nil
	}
	return f.byID[consignmentID], nil
}

func TestGetConsignments_UnchangedWithoutCore(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "wf1", "PENDING", "t1")

	got, err := NewService(store, &fakeNSW{err: errors.New("core must not be called")}).GetConsignments(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("GetConsignments: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ConsignmentID != "wf1" || got.Items[0].TaskCount != 1 {
		t.Fatalf("unexpected list: %+v", got.Items)
	}
	if got.Items[0].ConsignmentName != "" || got.Items[0].ConsigneeName != "" {
		t.Errorf("list must not call Core: %+v", got.Items[0])
	}
}

func TestGetConsignment_AddsNamesFromCore(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "wf1", "PENDING", "t1")
	nsw := &fakeNSW{byID: map[string]*nswclient.ConsignmentAgency{
		"wf1": {ConsignmentName: "Named", ConsigneeName: "Consignee"},
	}}
	svc := NewService(store, nsw)

	got, err := svc.GetConsignment(context.Background(), "wf1")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.ConsigneeName != "Consignee" || got.ConsignmentName != "Named" || got.TaskCount != 1 {
		t.Errorf("unexpected summary: %+v", got)
	}

	if _, err := svc.GetConsignment(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetConsignment_DropsPlaceholders(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "wf1", "PENDING", "t1")
	nsw := &fakeNSW{byID: map[string]*nswclient.ConsignmentAgency{
		"wf1": {ConsignmentName: "N/A", ConsigneeName: " n/a "},
	}}
	got, err := NewService(store, nsw).GetConsignment(context.Background(), "wf1")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.ConsignmentName != "" || got.ConsigneeName != "" {
		t.Errorf("placeholders must be omitted, got %+v", got)
	}
}

func TestGetConsignment_CoreFailureReturnsStoreRow(t *testing.T) {
	store := newTestStore(t)
	seedConsignment(t, store, "wf1", "PENDING", "t1")
	svc := NewService(store, &fakeNSW{err: errors.New("core down")})

	got, err := svc.GetConsignment(context.Background(), "wf1")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.ConsignmentID != "wf1" || got.ConsignmentName != "" {
		t.Errorf("expected store row without names, got %+v", got)
	}
}
