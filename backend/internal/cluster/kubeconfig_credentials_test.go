package cluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// kubeconfigWithUser wraps one `users:` entry in an otherwise valid kubeconfig
// pointing at server, so tests only vary the credential shape.
func kubeconfigWithUser(server, user string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: only
clusters:
- name: only-cluster
  cluster:
    server: %s
    insecure-skip-tls-verify: true
contexts:
- name: only
  context:
    cluster: only-cluster
    user: only-user
users:
- name: only-user
  user:
%s
`, server, user)
}

// Credentials that make Omastx run a command or open a local file must never
// reach client-go: an uploaded kubeconfig is untrusted, so an exec plugin is
// remote code execution on the backend and a file path is an arbitrary read of
// the backend's filesystem. Static credentials must keep working.
func TestUnsupportedCredentialsAreRejected(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		wantErr string // "" means the credentials must be accepted
	}{
		{
			name: "exec credential plugin",
			user: "    exec:\n" +
				"      apiVersion: client.authentication.k8s.io/v1beta1\n" +
				"      command: /bin/sh\n" +
				"      interactiveMode: Never\n" +
				"      args: [\"-c\", \"touch /tmp/omastx-rce\"]",
			wantErr: "exec credential plugin",
		},
		{
			name:    "auth-provider plugin",
			user:    "    auth-provider:\n      name: gcp",
			wantErr: `"gcp" auth-provider plugin`,
		},
		{
			name:    "bearer token from file path",
			user:    "    tokenFile: /proc/self/environ",
			wantErr: "bearer token from a file path",
		},
		{
			name:    "client certificate from file path",
			user:    "    client-certificate: /etc/omastx/tls.crt\n    client-key: /etc/omastx/tls.key",
			wantErr: "client certificate from a file path",
		},
		{
			name:    "client key from file path",
			user:    "    client-key: /etc/omastx/tls.key",
			wantErr: "client certificate from a file path",
		},
		{
			name: "static bearer token",
			user: "    token: sha256~static-bearer-token",
		},
		{
			name: "embedded client certificate data",
			user: "    client-certificate-data: dGVzdA==\n    client-key-data: dGVzdA==",
		},
		{
			name: "basic auth",
			user: "    username: viewer\n    password: hunter2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfig := []byte(kubeconfigWithUser("https://127.0.0.1:6443", tt.user))

			contexts, err := ListContexts(kubeconfig)
			if err != nil {
				t.Fatalf("ListContexts: %v", err)
			}
			if len(contexts) != 1 {
				t.Fatalf("contexts = %d, want 1", len(contexts))
			}
			if tt.wantErr == "" {
				if contexts[0].Unsupported != "" {
					t.Errorf("context flagged unsupported: %q", contexts[0].Unsupported)
				}
			} else if !strings.Contains(contexts[0].Unsupported, tt.wantErr) {
				t.Errorf("unsupported = %q, want containing %q", contexts[0].Unsupported, tt.wantErr)
			}

			_, err = RESTConfig(kubeconfig, "only")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("supported credentials rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("RESTConfig accepted %s; expected rejection", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
			// The message must name the accepted alternative (SPEC §4.6).
			if !strings.Contains(err.Error(), "bearer token") {
				t.Errorf("err = %v, want the remedy hint", err)
			}
		})
	}
}

// A cluster whose certificate authority is a file path would read that file from
// the Omastx host, so it is refused like the credential file paths.
func TestCertificateAuthorityFileRejected(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
current-context: only
clusters:
- name: only-cluster
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority: /etc/omastx/ca.crt
contexts:
- name: only
  context: {cluster: only-cluster, user: only-user}
users:
- name: only-user
  user:
    token: static
`)
	_, err := RESTConfig(kubeconfig, "only")
	if err == nil || !strings.Contains(err.Error(), "certificate authority from a file path") {
		t.Fatalf("err = %v, want certificate-authority rejection", err)
	}
}

// A kubeconfig mixing cloud contexts with a static one (the usual ~/.kube/config)
// must still list every context, flagging only the ones that can't be imported.
func TestListContextsFlagsOnlyUnsupportedContexts(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
current-context: eks
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: eks
  context: {cluster: c, user: eks-user}
- name: static
  context: {cluster: c, user: static-user}
users:
- name: eks-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws-iam-authenticator
      interactiveMode: Never
- name: static-user
  user:
    token: static
`)
	contexts, err := ListContexts(kubeconfig)
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts = %d, want 2", len(contexts))
	}
	byName := map[string]ContextInfo{}
	for _, c := range contexts {
		byName[c.Name] = c
	}
	if byName["eks"].Unsupported == "" {
		t.Error("exec-plugin context must be flagged unsupported")
	}
	if got := byName["static"].Unsupported; got != "" {
		t.Errorf("static-token context flagged unsupported: %q", got)
	}
	if _, err := RESTConfig(kubeconfig, "static"); err != nil {
		t.Errorf("importable context rejected: %v", err)
	}
}

// End to end through the Connector with a reachable server: the credential
// plugin must not run and nothing may reach the target server.
func TestKubeConnectorCheckRunsNoCredentialPlugin(t *testing.T) {
	requests := make(chan string, 4)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		_, _ = w.Write([]byte(`{"major":"1","minor":"30","gitVersion":"v1.30.0"}`))
	}))
	defer srv.Close()

	marker := filepath.Join(t.TempDir(), "pwned")
	kubeconfig := []byte(kubeconfigWithUser(srv.URL,
		"    exec:\n"+
			"      apiVersion: client.authentication.k8s.io/v1beta1\n"+
			"      command: /bin/sh\n"+
			"      interactiveMode: Never\n"+
			fmt.Sprintf("      args: [\"-c\", \"echo owned > %s\"]", marker)))

	conn := &KubeConnector{Timeout: 5 * time.Second}
	if _, err := conn.Check(context.Background(), kubeconfig, "only"); err == nil {
		t.Fatal("Check accepted an exec-plugin kubeconfig; expected rejection")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("remote code execution: the exec plugin ran during Check")
	}
	select {
	case path := <-requests:
		t.Fatalf("rejected kubeconfig still reached the server at %s", path)
	default:
	}
}
