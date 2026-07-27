package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An uploaded kubeconfig names the API server, so connecting to a cluster must
// not be usable to reach addresses that are never API servers - above all the
// cloud metadata service, which would hand out the node's IAM credentials.
func TestRESTConfigGuardsOutboundConnections(t *testing.T) {
	restCfg, err := RESTConfig([]byte(twoContextKubeconfig), "staging")
	if err != nil {
		t.Fatalf("RESTConfig: %v", err)
	}
	if restCfg.Dial == nil {
		t.Fatal("rest.Config.Dial is nil: outbound connections would be unguarded")
	}
	if _, err := restCfg.Dial(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Error("dialed the cloud metadata service; want it blocked")
	} else if !strings.Contains(err.Error(), "refusing to connect") {
		t.Errorf("err = %v, want the guard's refusal", err)
	}
}

// End to end: a kubeconfig whose server points at a blocked address must fail the
// connection check without any request reaching it.
func TestKubeConnectorCheckRefusesBlockedServer(t *testing.T) {
	reached := make(chan struct{}, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached <- struct{}{}
		_, _ = w.Write([]byte(`{"major":"1","minor":"30","gitVersion":"v1.30.0"}`))
	}))
	defer srv.Close()

	kubeconfig := []byte(kubeconfigWithUser(srv.URL, "    token: static"))
	result, err := (&KubeConnector{Timeout: 5 * time.Second}).
		Check(context.Background(), kubeconfig, "only")
	if err != nil {
		t.Fatalf("Check returned a hard error: %v", err)
	}
	if result.Reachable {
		t.Fatal("loopback server reported reachable; the guard did not apply")
	}
	if !strings.Contains(result.Error, "refusing to connect") {
		t.Errorf("error = %q, want the guard's refusal", result.Error)
	}
	select {
	case <-reached:
		t.Error("the blocked server was contacted")
	default:
	}
}
