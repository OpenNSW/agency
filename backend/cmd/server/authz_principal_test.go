package main

import (
	"reflect"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/authn"
)

func TestAuthzPrincipal_Subject(t *testing.T) {
	tests := []struct {
		name string
		p    *authn.Principal
		want string
	}{
		{
			name: "user principal returns UserID",
			p:    &authn.Principal{Kind: authn.KindUser, UserID: "user-1", ClientID: "should-not-be-used"},
			want: "user-1",
		},
		{
			name: "client principal returns ClientID",
			p:    &authn.Principal{Kind: authn.KindClient, ClientID: "NSW_TO_NPQS"},
			want: "NSW_TO_NPQS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authzPrincipal{tt.p}.Subject()
			if got != tt.want {
				t.Fatalf("Subject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthzPrincipal_RolesAndScopes(t *testing.T) {
	p := &authn.Principal{
		Kind:   authn.KindUser,
		UserID: "user-1",
		Roles:  []string{"OGA Reviewer"},
		Scopes: []string{"agency:application:read", "agency:profile:read"},
	}
	a := authzPrincipal{p}

	if !reflect.DeepEqual(a.Roles(), p.Roles) {
		t.Fatalf("Roles() = %v, want %v", a.Roles(), p.Roles)
	}
	if !reflect.DeepEqual(a.Scopes(), p.Scopes) {
		t.Fatalf("Scopes() = %v, want %v", a.Scopes(), p.Scopes)
	}
}
