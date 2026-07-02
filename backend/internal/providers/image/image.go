// Package image implements core.ArtifactProvider for container images. It walks
// all workload kinds (Deployments, StatefulSets, DaemonSets, CronJobs, bare Pods),
// including init and ephemeral containers, and normalizes each image reference to a
// registry-qualified identity (SPEC §1.4, §2.2). Read-only: only List calls.
package image

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banshee86vr/omastx/backend/internal/core"
)

// Provider discovers container images running in a cluster.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Kind() string { return "image" }

func (p *Provider) Discover(ctx context.Context, c core.ClusterClient) ([]core.Artifact, error) {
	cs := c.Clientset()
	acc := &accumulator{seen: map[string]bool{}}
	var errs []error

	if deps, err := cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{}); err != nil {
		errs = append(errs, fmt.Errorf("list deployments: %w", err))
	} else {
		for i := range deps.Items {
			d := &deps.Items[i]
			acc.addPodSpec("Deployment", d.Name, d.Namespace, &d.Spec.Template.Spec)
		}
	}

	if sts, err := cs.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{}); err != nil {
		errs = append(errs, fmt.Errorf("list statefulsets: %w", err))
	} else {
		for i := range sts.Items {
			s := &sts.Items[i]
			acc.addPodSpec("StatefulSet", s.Name, s.Namespace, &s.Spec.Template.Spec)
		}
	}

	if ds, err := cs.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{}); err != nil {
		errs = append(errs, fmt.Errorf("list daemonsets: %w", err))
	} else {
		for i := range ds.Items {
			d := &ds.Items[i]
			acc.addPodSpec("DaemonSet", d.Name, d.Namespace, &d.Spec.Template.Spec)
		}
	}

	if cjs, err := cs.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{}); err != nil {
		errs = append(errs, fmt.Errorf("list cronjobs: %w", err))
	} else {
		for i := range cjs.Items {
			cj := &cjs.Items[i]
			acc.addPodSpec("CronJob", cj.Name, cj.Namespace, &cj.Spec.JobTemplate.Spec.Template.Spec)
		}
	}

	// Bare pods only: pods with an owner are already covered by their controller.
	if pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err != nil {
		errs = append(errs, fmt.Errorf("list pods: %w", err))
	} else {
		for i := range pods.Items {
			pod := &pods.Items[i]
			if len(pod.OwnerReferences) > 0 {
				continue
			}
			acc.addPodSpec("Pod", pod.Name, pod.Namespace, &pod.Spec)
		}
	}

	return acc.artifacts, errors.Join(errs...)
}

type accumulator struct {
	artifacts []core.Artifact
	// seen dedups identical (owner, identity) so an image used by an init and a
	// main container in the same workload is counted once.
	seen map[string]bool
}

func (a *accumulator) addPodSpec(ownerKind, ownerName, namespace string, spec *corev1.PodSpec) {
	add := func(img string) {
		if img == "" {
			return
		}
		identity, installed, registry, ok := parseImage(img)
		if !ok {
			return
		}
		key := ownerKind + "/" + namespace + "/" + ownerName + "/" + identity
		if a.seen[key] {
			return
		}
		a.seen[key] = true
		a.artifacts = append(a.artifacts, core.Artifact{
			Kind:      "image",
			Namespace: namespace,
			OwnerKind: ownerKind,
			OwnerName: ownerName,
			Identity:  identity,
			Installed: installed,
			SourceMeta: map[string]any{
				"registry": registry,
				"image":    img,
			},
		})
	}
	for _, ct := range spec.InitContainers {
		add(ct.Image)
	}
	for _, ct := range spec.Containers {
		add(ct.Image)
	}
	for _, ct := range spec.EphemeralContainers {
		add(ct.Image)
	}
}

// parseImage normalizes a container image reference into a registry-qualified
// identity and the installed tag/digest. Docker Hub's implicit registry and
// library namespace are made explicit ("nginx" → docker.io/library/nginx).
func parseImage(image string) (identity, installed, registry string, ok bool) {
	ref, err := name.ParseReference(image, name.WeakValidation)
	if err != nil {
		return "", "", "", false
	}
	repo := ref.Context()
	registry = repo.RegistryStr()
	identity = repo.Name()
	if registry == name.DefaultRegistry {
		registry = "docker.io"
		identity = "docker.io/" + repo.RepositoryStr()
	}
	switch r := ref.(type) {
	case name.Tag:
		installed = r.TagStr()
	case name.Digest:
		installed = r.DigestStr()
	}
	return identity, installed, registry, true
}
