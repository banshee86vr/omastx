// Package cluster handles kubeconfig parsing, connection checks, and the
// read-only RBAC self-check (SPEC §2.6). Kubeconfig bytes are only ever held
// in memory here; persistence (encrypted) is the store's job.
package cluster

import (
	"fmt"
	"sort"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ContextInfo describes one context found in an uploaded kubeconfig, so the
// user can choose which ones to import.
type ContextInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	Server  string `json:"server"`
	User    string `json:"user"`
	Current bool   `json:"current"`
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
	if _, ok := raw.Contexts[contextName]; !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}
	restCfg, err := clientcmd.NewNonInteractiveClientConfig(raw, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config for context %q: %w", contextName, err)
	}
	return restCfg, nil
}
