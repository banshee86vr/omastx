package oci

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The registry host comes from an image reference read out of a connected
// cluster, so listing tags must not be usable to reach addresses that are never
// registries (SSRF).
func TestRemoteListerGuardsOutboundConnections(t *testing.T) {
	reached := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Run("guarded transport refuses the address", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := guardedTransport.RoundTrip(req)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatal("guarded transport reached a blocked address")
		}
		if !strings.Contains(err.Error(), "refusing to connect") {
			t.Errorf("err = %v, want the guard's refusal", err)
		}
	})

	// The lister must use that transport. Registry clients retry hard on
	// connection errors, so this is bounded by the context; what matters is that
	// it fails and the address is never contacted.
	t.Run("lister uses it", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		host := strings.TrimPrefix(srv.URL, "http://")
		if _, err := (remoteLister{}).List(ctx, host+"/library/app", nil); err == nil {
			t.Error("listed tags from a blocked address; want it refused")
		}
	})

	select {
	case <-reached:
		t.Error("the blocked address was contacted")
	default:
	}
}
