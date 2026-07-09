// Package helm implements core.ArtifactProvider for Helm 3 releases stored as
// Secrets (label owner=helm). Release payloads are decoded in memory with
// helm.sh/helm/v3; only chart name/version/repo metadata is persisted (SPEC §2.6).
package helm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banshee86vr/omastx/backend/internal/core"
)

// Provider discovers deployed Helm 3 releases from release Secrets.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Kind() string { return "helm" }

func (p *Provider) Discover(ctx context.Context, c core.ClusterClient) ([]core.Artifact, error) {
	cs := c.Clientset()
	secrets, err := cs.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm,status=deployed",
	})
	if err != nil {
		return nil, fmt.Errorf("list helm release secrets: %w", err)
	}

	var artifacts []core.Artifact
	var errs []error
	for i := range secrets.Items {
		sec := &secrets.Items[i]
		a, err := artifactFromSecret(sec)
		if err != nil {
			errs = append(errs, fmt.Errorf("namespace %s secret %s: %w", sec.Namespace, sec.Name, err))
			continue
		}
		if a != nil {
			artifacts = append(artifacts, *a)
		}
	}
	return artifacts, errors.Join(errs...)
}

func artifactFromSecret(sec *corev1.Secret) (*core.Artifact, error) {
	raw, ok := sec.Data["release"]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	rel, err := decodeRelease(string(raw))
	if err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if rel.Chart == nil || rel.Chart.Metadata == nil {
		return nil, nil
	}
	meta := rel.Chart.Metadata
	if meta.Name == "" || meta.Version == "" {
		return nil, nil
	}

	releaseName := rel.Name
	if releaseName == "" {
		releaseName = sec.Labels["name"]
	}

	sourceMeta := map[string]any{
		"release":     releaseName,
		"chart":       meta.Name,
		"app_version": meta.AppVersion,
	}
	if meta.Home != "" {
		sourceMeta["home"] = meta.Home
	}
	if len(meta.Sources) > 0 {
		sourceMeta["sources"] = meta.Sources
	}
	if repo := chartRepoURL(meta); repo != "" {
		sourceMeta["chart_repo"] = repo
	}
	if len(meta.Maintainers) > 0 {
		names := make([]string, 0, len(meta.Maintainers))
		for _, m := range meta.Maintainers {
			if m.Name != "" {
				names = append(names, m.Name)
			}
		}
		if len(names) > 0 {
			sourceMeta["maintainers"] = names
		}
	}

	return &core.Artifact{
		Kind:       "helm",
		Namespace:  sec.Namespace,
		OwnerKind:  "HelmRelease",
		OwnerName:  releaseName,
		Identity:   meta.Name,
		Installed:  meta.Version,
		SourceMeta: sourceMeta,
	}, nil
}

// chartRepoURL extracts the Helm repository URL from chart metadata when present.
func chartRepoURL(meta *chart.Metadata) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{
		"artifacthub.io/repository",
		"catalog.cattle.io/ui-source-repo",
	} {
		if v := strings.TrimSpace(meta.Annotations[key]); v != "" {
			return v
		}
	}
	for _, s := range meta.Sources {
		if looksLikeRepoURL(s) {
			return s
		}
	}
	return ""
}

func looksLikeRepoURL(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "oci://") {
		return len(s) > len("oci://")
	}
	return len(s) >= 7 && (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://"))
}
