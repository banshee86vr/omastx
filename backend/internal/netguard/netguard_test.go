package netguard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// allowLoopbackFor flips the deployment policy for one test and restores it.
func allowLoopbackFor(t *testing.T, allow bool) {
	t.Helper()
	prev := allowLoopback.Load()
	AllowLoopback(allow)
	t.Cleanup(func() { AllowLoopback(prev) })
}

func TestCheckAddr(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr string // "" means the address must be dialable
	}{
		// The whole point: cloud metadata must be unreachable however it is spelled.
		{"AWS/GCP/Azure metadata", "169.254.169.254:80", "link-local"},
		{"metadata via IPv4-mapped IPv6", "[::ffff:169.254.169.254]:80", "link-local"},
		{"AWS IPv6 metadata", "[fd00:ec2::254]:80", "cloud metadata service"},
		{"IPv6 link-local", "[fe80::1]:6443", "link-local"},
		{"other link-local", "169.254.1.1:6443", "link-local"},
		{"unspecified v4", "0.0.0.0:6443", "not a routable host"},
		{"unspecified v6", "[::]:6443", "not a routable host"},
		{"link-local multicast", "224.0.0.1:6443", "link-local"},
		{"multicast", "239.1.2.3:6443", "not a routable host"},
		{"interface-local multicast", "[ff01::1]:6443", "not a routable host"},
		{"loopback v4", "127.0.0.1:6443", "loopback"},
		{"loopback alias", "127.1.2.3:6443", "loopback"},
		{"loopback v6", "[::1]:6443", "loopback"},
		{"not an IP", "metadata.google.internal:80", "not an IP address"},
		{"no port", "169.254.169.254", "unrecognised address"},

		// Private ranges stay reachable: on-prem clusters and internal
		// registries are the normal deployment, not an attack.
		{"RFC1918 ten", "10.1.2.3:6443", ""},
		{"RFC1918 192.168", "192.168.1.10:6443", ""},
		{"RFC1918 172.16", "172.16.0.5:6443", ""},
		{"unique local v6", "[fd12::1]:6443", ""},
		{"carrier NAT / VPN", "100.64.0.1:6443", ""},
		{"public", "8.8.8.8:443", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAddr(tt.address)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkAddr(%q) = %v, want allowed", tt.address, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkAddr(%q) allowed the address; want it blocked", tt.address)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoopbackAllowedInDevOnly(t *testing.T) {
	allowLoopbackFor(t, true)
	if err := checkAddr("127.0.0.1:6443"); err != nil {
		t.Fatalf("dev mode must allow loopback: %v", err)
	}
	// Metadata stays blocked even in dev mode - it is never a scan target.
	if err := checkAddr("169.254.169.254:80"); err == nil {
		t.Error("dev mode must not open up the metadata service")
	}
}

// The guard has to bite at the resolved IP rather than the URL, so a hostname
// pointing at a blocked address is refused too (this is what makes DNS
// rebinding useless).
func TestTransportBlocksHostnameResolvingToBlockedIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached"))
	}))
	defer srv.Close()
	url := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	client := &http.Client{Transport: Transport()}

	allowLoopbackFor(t, false)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to a hostname resolving to loopback succeeded; want it blocked")
	}
	if !strings.Contains(err.Error(), "refusing to connect") {
		t.Errorf("err = %v, want the guard's refusal", err)
	}

	// Same request must succeed once the policy allows loopback, proving the
	// transport is otherwise functional.
	allowLoopbackFor(t, true)
	resp, err = client.Do(req.Clone(context.Background()))
	if err != nil {
		t.Fatalf("loopback allowed but request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
