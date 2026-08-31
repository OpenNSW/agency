package consignment

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/internal/datascope"
	"github.com/OpenNSW/agency/backend/internal/nswclient"
)

// unrestrictedResolver returns a datascope.Resolver with no configured
// rules, so Resolve always reports Unrestricted without needing a real
// UserAttributes implementation.
func unrestrictedResolver() *datascope.Resolver {
	return datascope.NewResolver(nil, nil)
}

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

	got, err := NewService(store, &stubNSWClient{}, unrestrictedResolver()).GetConsignments(context.Background(), "", 1, 10)
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

	got, err := NewService(store, &stubNSWClient{}, unrestrictedResolver()).GetConsignments(context.Background(), "", 1, 10)
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
	svc := NewService(store, nsw, unrestrictedResolver())
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
	svc := NewService(store, nsw, unrestrictedResolver())
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
	svc := NewService(store, &stubNSWClient{}, unrestrictedResolver())

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

// stubUserAttributes is a minimal datascope.UserAttributes for tests.
type stubUserAttributes struct {
	data map[string]any
}

func (s stubUserAttributes) GetCustomData(_ context.Context, _ string) (map[string]any, error) {
	return s.data, nil
}

func userContext() context.Context {
	return authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindUser, UserID: "user-1"})
}

func clientContext() context.Context {
	return authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindClient, ClientID: "client-1"})
}

func TestGetConsignments_UnsatisfiableScope_ReturnsEmptyPageWithoutQuerying(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ADAM PVT LTD"}, "t1")

	rules := []datascope.Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	resolver := datascope.NewResolver(rules, stubUserAttributes{data: map[string]any{}}) // no assignedDistrict set

	svc := NewService(store, &stubNSWClient{}, resolver)
	got, err := svc.GetConsignments(userContext(), "", 1, 10)
	if err != nil {
		t.Fatalf("GetConsignments: %v", err)
	}
	if got.Total != 0 || len(got.Items) != 0 {
		t.Errorf("expected empty page for an unsatisfiable scope, got %+v", got)
	}
}

func TestGetConsignments_ClientPrincipalBypassesScoping(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ADAM PVT LTD"}, "t1")

	rules := []datascope.Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	resolver := datascope.NewResolver(rules, stubUserAttributes{data: map[string]any{}})

	svc := NewService(store, &stubNSWClient{}, resolver)
	got, err := svc.GetConsignments(clientContext(), "", 1, 10)
	if err != nil {
		t.Fatalf("GetConsignments: %v", err)
	}
	if got.Total != 1 {
		t.Errorf("expected a client principal to bypass scoping and see everything, got %+v", got)
	}
}

func TestGetConsignment_ScopeMismatch_ReturnsNotFound(t *testing.T) {
	store := newTestStore(t)
	seedConsignmentWithCustomData(t, store, "c-gampaha", "PENDING", JSONB{"location": JSONB{"district": "Gampaha"}})

	rules := []datascope.Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	resolver := datascope.NewResolver(rules, stubUserAttributes{data: map[string]any{"assignedDistrict": "Colombo"}})

	svc := NewService(store, &stubNSWClient{}, resolver)
	_, err := svc.GetConsignment(userContext(), "c-gampaha")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetConsignment error = %v, want ErrNotFound for an out-of-scope consignment", err)
	}
}
