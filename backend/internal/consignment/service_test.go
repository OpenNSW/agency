package consignment

import (
	"context"
	"errors"
	"testing"
)

func TestGetConsignments_ReturnsLocalStoreData(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ADAM PVT LTD"}, "t1")

	got, err := NewService(store).GetConsignments(context.Background(), "", 1, 10)
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

	got, err := NewService(store).GetConsignments(context.Background(), "", 1, 10)
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

func TestGetConsignment_ReturnsLocalStoreData(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ADAM PVT LTD"}, "t1")
	svc := NewService(store)

	got, err := svc.GetConsignment(context.Background(), "wf1")
	if err != nil {
		t.Fatalf("GetConsignment: %v", err)
	}
	if got.TraderCompanyName != "ADAM PVT LTD" || got.TaskCount != 1 {
		t.Errorf("unexpected summary: %+v", got)
	}

	if _, err := svc.GetConsignment(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
