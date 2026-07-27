package helmrepo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Chart repository URLs come from Helm release metadata inside a connected
// cluster, so the default fetcher must not be usable to reach addresses that are
// never chart repositories (SSRF).
func TestDefaultFetcherGuardsOutboundConnections(t *testing.T) {
	reached := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached <- struct{}{}
		_, _ = w.Write([]byte("entries: {}\n"))
	}))
	defer srv.Close()

	_, err := New(nil).fetcher.Fetch(context.Background(), srv.URL, "any-chart")
	if err == nil {
		t.Fatal("fetched an index from a blocked address; want it refused")
	}
	if !strings.Contains(err.Error(), "refusing to connect") {
		t.Errorf("err = %v, want the guard's refusal", err)
	}
	select {
	case <-reached:
		t.Error("the blocked address was contacted")
	default:
	}
}
