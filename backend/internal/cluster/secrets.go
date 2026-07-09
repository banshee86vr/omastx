package cluster

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/banshee86vr/omastx/backend/internal/registryauth"
)

// AccessibleSecret is a Kubernetes secret the connected identity can read.
// Only metadata and data key names are returned — never secret values (SPEC §2.6).
// Registries lists hostnames from dockerconfig auths keys when present.
type AccessibleSecret struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Keys       []string `json:"keys"`
	Registries []string `json:"registries,omitempty"`
}

// ListAccessibleSecrets discovers secrets visible to the kubeconfig identity.
// It merges cluster-wide and per-namespace lists so namespace-scoped grants still appear.
func ListAccessibleSecrets(ctx context.Context, client kubernetes.Interface) ([]AccessibleSecret, error) {
	byRef := map[string]AccessibleSecret{}

	add := func(items []corev1.Secret) {
		for i := range items {
			sec := items[i]
			if sec.Namespace == "" || sec.Name == "" {
				continue
			}
			ref := sec.Namespace + "/" + sec.Name
			keys := secretKeyNames(&sec)
			if len(keys) == 0 {
				continue
			}
			registries := registriesFromSecret(&sec)
			if existing, ok := byRef[ref]; ok {
				keys = mergeKeyNames(existing.Keys, keys)
				registries = mergeRegistryHosts(existing.Registries, registries)
			}
			byRef[ref] = AccessibleSecret{
				Namespace:  sec.Namespace,
				Name:       sec.Name,
				Keys:       keys,
				Registries: registries,
			}
		}
	}

	clusterForbidden := false
	if all, err := client.CoreV1().Secrets("").List(ctx, metav1.ListOptions{}); err == nil {
		add(all.Items)
	} else if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		clusterForbidden = true
	} else {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if clusterForbidden {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		// Cluster-wide list worked; namespace enumeration is best-effort.
		return sortAccessibleSecrets(byRef), nil
	}

	for _, ns := range nsList.Items {
		secrets, err := client.CoreV1().Secrets(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				continue
			}
			return nil, fmt.Errorf("list secrets in %q: %w", ns.Name, err)
		}
		add(secrets.Items)
	}

	if len(byRef) == 0 && clusterForbidden {
		return nil, fmt.Errorf("no accessible secrets: missing get/list on secrets")
	}
	return sortAccessibleSecrets(byRef), nil
}

func secretKeyNames(sec *corev1.Secret) []string {
	if len(sec.Data) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sec.Data))
	for k := range sec.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func registriesFromSecret(sec *corev1.Secret) []string {
	if sec == nil || len(sec.Data) == 0 {
		return nil
	}
	var hosts []string
	if raw, ok := sec.Data[corev1.DockerConfigJsonKey]; ok {
		hosts = append(hosts, registryauth.RegistryHostsFromDockerConfigJSON(raw)...)
	}
	if raw, ok := sec.Data[corev1.DockerConfigKey]; ok {
		hosts = append(hosts, registryauth.RegistryHostsFromDockerConfigJSON(raw)...)
	}
	return mergeRegistryHosts(nil, hosts)
}

func mergeRegistryHosts(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, h := range append(a, b...) {
		if h == "" {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func mergeKeyNames(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, k := range append(a, b...) {
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortAccessibleSecrets(byRef map[string]AccessibleSecret) []AccessibleSecret {
	out := make([]AccessibleSecret, 0, len(byRef))
	for _, sec := range byRef {
		out = append(out, sec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return false
	})
	return out
}
