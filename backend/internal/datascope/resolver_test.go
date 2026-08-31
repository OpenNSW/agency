package datascope

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/authn"
)

type stubUserAttributes struct {
	data map[string]any
	err  error
}

func (s stubUserAttributes) GetCustomData(ctx context.Context, userID string) (map[string]any, error) {
	return s.data, s.err
}

func userContext() context.Context {
	return authn.ContextWithPrincipal(context.Background(), &authn.Principal{
		Kind:   authn.KindUser,
		UserID: "user-1",
	})
}

func clientContext() context.Context {
	return authn.ContextWithPrincipal(context.Background(), &authn.Principal{
		Kind:     authn.KindClient,
		ClientID: "client-1",
	})
}

func TestResolve_NoRulesConfigured(t *testing.T) {
	r := NewResolver(nil, stubUserAttributes{})
	res, err := r.Resolve(userContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Unrestricted {
		t.Errorf("Resolution = %+v, want Unrestricted", res)
	}
}

func TestResolve_ClientPrincipalBypassesScoping(t *testing.T) {
	rules := []Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	r := NewResolver(rules, stubUserAttributes{})
	res, err := r.Resolve(clientContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Unrestricted {
		t.Errorf("Resolution = %+v, want Unrestricted for a client principal", res)
	}
}

func TestResolve_NoPrincipalBypassesScoping(t *testing.T) {
	rules := []Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	r := NewResolver(rules, stubUserAttributes{})
	res, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Unrestricted {
		t.Errorf("Resolution = %+v, want Unrestricted with no principal in context", res)
	}
}

func TestResolve_Satisfiable(t *testing.T) {
	rules := []Rule{
		{ConsignmentField: "/location/district", UserField: "/assignedDistrict"},
		{ConsignmentField: "/priority", UserField: "/level"},
	}
	attrs := stubUserAttributes{data: map[string]any{
		"assignedDistrict": "Colombo",
		"level":            float64(2),
	}}
	r := NewResolver(rules, attrs)
	res, err := r.Resolve(userContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Unrestricted {
		t.Fatal("Resolution.Unrestricted = true, want false")
	}
	if !res.Satisfiable {
		t.Fatal("Resolution.Satisfiable = false, want true")
	}
	want := map[string]any{
		"/location/district": "Colombo",
		"/priority":          float64(2),
	}
	if !reflect.DeepEqual(res.Filter, want) {
		t.Errorf("Filter = %v, want %v", res.Filter, want)
	}
}

func TestResolve_UnsatisfiableWhenAttributeMissing(t *testing.T) {
	rules := []Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	attrs := stubUserAttributes{data: map[string]any{"otherField": "x"}}
	r := NewResolver(rules, attrs)
	res, err := r.Resolve(userContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Satisfiable {
		t.Error("Resolution.Satisfiable = true, want false (attribute not set on this officer)")
	}
}

func TestResolve_UnsatisfiableWhenCustomDataNil(t *testing.T) {
	rules := []Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	r := NewResolver(rules, stubUserAttributes{data: nil})
	res, err := r.Resolve(userContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Satisfiable {
		t.Error("Resolution.Satisfiable = true, want false (nil custom data)")
	}
}

func TestResolve_PropagatesInfraError(t *testing.T) {
	rules := []Rule{{ConsignmentField: "/location/district", UserField: "/assignedDistrict"}}
	wantErr := errors.New("db is down")
	r := NewResolver(rules, stubUserAttributes{err: wantErr})
	_, err := r.Resolve(userContext())
	if err == nil {
		t.Fatal("Resolve returned nil error, want the underlying infra error to propagate")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}
