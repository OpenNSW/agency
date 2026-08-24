package nswclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/agency/backend/pkg/httpclient"
)

func TestClient_GetConsignmentAgency(t *testing.T) {
	const id = "cefda05e-3071-4e94-b001-328094e570a7"

	mux := http.NewServeMux()
	mux.HandleFunc("/consignments/"+id+"/agency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"consignmentId":"` + id + `",
			"traderCompanyName":"ADAM PVT LTD",
			"email":"must-be-ignored@example.com",
			"phone":"+9411"
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewWithClient(httpclient.NewClientBuilder().WithBaseURL(server.URL + "/").Build())
	got, err := client.GetConsignmentAgency(context.Background(), id)
	if err != nil {
		t.Fatalf("GetConsignmentAgency: %v", err)
	}
	if got.ConsignmentID != id {
		t.Errorf("consignmentId: got %q", got.ConsignmentID)
	}
	if got.TraderCompanyName != "ADAM PVT LTD" {
		t.Errorf("traderCompanyName: got %q", got.TraderCompanyName)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "email") || strings.Contains(string(raw), "phone") || strings.Contains(string(raw), "must-be-ignored") {
		t.Errorf("marshaled DTO leaked contact fields: %s", raw)
	}
}

func TestClient_GetConsignmentAgency_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/consignments/missing/agency", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewWithClient(httpclient.NewClientBuilder().WithBaseURL(server.URL + "/").Build())
	if _, err := client.GetConsignmentAgency(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestClient_GetConsignmentAgency_EmptyID(t *testing.T) {
	client := NewWithClient(httpclient.NewClientBuilder().Build())
	if _, err := client.GetConsignmentAgency(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty id")
	}
}
