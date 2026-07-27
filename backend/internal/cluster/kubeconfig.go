// Package cluster handles kubeconfig parsing, connection checks, and the
// read-only RBAC self-check (SPEC §2.6). Kubeconfig bytes are only ever held
// in memory here; persistence (encrypted) is the store's job.
package cluster

import (
	"fmt"
	"sort"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/banshee86vr/omastx/backend/internal/netguard"
)

// credentialHint is the remedy offered with every unsupported-credential
// rejection, so the message states cause + next step (SPEC §4.6).
const credentialHint = "Omastx only accepts credentials embedded in the kubeconfig: " +
	"run \"kubectl config view --raw --flatten\" to inline certificate files, " +
	"or use a service account bearer token."

// unsupportedCredentials reports why a context's credentials cannot be resolved
// by Omastx, or "" when they can.
//
// An uploaded kubeconfig is untrusted input that this process resolves on the
// server, so anything indirect is refused before client-go authenticates:
// exec/auth-provider plugins would run an uploader-chosen command on this host
// (remote code execution), and a file path would be read from this host's
// filesystem - not the uploader's - turning the connect flow into an arbitrary
// file read. Static credentials are also the only kind the distroless runtime
// can use, so nothing that ever worked is lost.
func unsupportedCredentials(authInfo *clientcmdapi.AuthInfo, clusterCfg *clientcmdapi.Cluster) string {
	if authInfo != nil {
		switch {
		case authInfo.Exec != nil:
			return "uses an exec credential plugin, which Omastx never runs"
		case authInfo.AuthProvider != nil:
			return fmt.Sprintf("uses the %q auth-provider plugin, which Omastx never runs",
				authInfo.AuthProvider.Name)
		case authInfo.TokenFile != "":
			return "reads its bearer token from a file path, which would resolve on the Omastx host"
		case authInfo.ClientCertificate != "" || authInfo.ClientKey != "":
			return "reads its client certificate from a file path, which would resolve on the Omastx host"
		}
	}
	if clusterCfg != nil && clusterCfg.CertificateAuthority != "" {
		return "reads its certificate authority from a file path, which would resolve on the Omastx host"
	}
	return ""
}

// ContextInfo describes one context found in an uploaded kubeconfig, so the
// user can choose which ones to import.
type ContextInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	Server  string `json:"server"`
	User    string `json:"user"`
	Current bool   `json:"current"`
	// Unsupported explains why this context cannot be imported, empty when it
	// can. Set for contexts whose credentials Omastx refuses to resolve.
	Unsupported string `json:"unsupported,omitempty"`
}

// ListContexts parses a kubeconfig and returns its contexts. It never mutates
// or stores anything.
func ListContexts(kubeconfig []byte) ([]ContextInfo, error) {
	cfg, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	if len(cfg.Contexts) == 0 {
		return nil, fmt.Errorf("kubeconfig contains no contexts")
	}
	contexts := make([]ContextInfo, 0, len(cfg.Contexts))
	for name, ctx := range cfg.Contexts {
		info := ContextInfo{
			Name:    name,
			Cluster: ctx.Cluster,
			User:    ctx.AuthInfo,
			Current: name == cfg.CurrentContext,
		}
		if c, ok := cfg.Clusters[ctx.Cluster]; ok {
			info.Server = c.Server
		}
		// Flagged, not dropped: a mixed kubeconfig (some cloud contexts, some
		// static ones) must still show why only part of it can be imported.
		info.Unsupported = unsupportedCredentials(cfg.AuthInfos[ctx.AuthInfo], cfg.Clusters[ctx.Cluster])
		contexts = append(contexts, info)
	}
	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].Current != contexts[j].Current {
			return contexts[i].Current
		}
		return contexts[i].Name < contexts[j].Name
	})
	return contexts, nil
}

// RESTConfig builds a rest.Config for one context of a kubeconfig.
func RESTConfig(kubeconfig []byte, contextName string) (*rest.Config, error) {
	cfg, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	raw, err := cfg.RawConfig()
	if err != nil {
		return nil, err
	}
	if contextName == "" {
		contextName = raw.CurrentContext
	}
	kubeCtx, ok := raw.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}
	// Must precede ClientConfig(): that call is where client-go would execute a
	// credential plugin or open a local file on this host.
	if reason := unsupportedCredentials(raw.AuthInfos[kubeCtx.AuthInfo], raw.Clusters[kubeCtx.Cluster]); reason != "" {
		return nil, fmt.Errorf("context %q %s. %s", contextName, reason, credentialHint)
	}
	restCfg, err := clientcmd.NewNonInteractiveClientConfig(raw, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config for context %q: %w", contextName, err)
	}
	// The server URL is uploader-controlled, so connections are guarded (SSRF).
	restCfg.Dial = netguard.DialContext
	return restCfg, nil
}

// Clientset builds a read-only clientset for one context of a kubeconfig, bounded
// by timeout. An empty contextName uses the kubeconfig's current-context.
func Clientset(kubeconfig []byte, contextName string, timeout time.Duration) (kubernetes.Interface, error) {
	restCfg, err := RESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		restCfg.Timeout = timeout
	}
	return kubernetes.NewForConfig(restCfg)
}
