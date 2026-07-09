// Package core holds the extensibility contract shared by the scan orchestrator,
// artifact providers, and version resolvers. Adding a new package kind (operators,
// nodes, ...) means implementing ArtifactProvider / VersionResolver here — never
// touching the orchestrator (SPEC §2.2, §8).
package core

import (
	"context"
	"time"

	"github.com/google/uuid"
	"k8s.io/client-go/kubernetes"
)

// ClusterClient is the read-only handle a provider gets for one cluster.
type ClusterClient interface {
	// ID is the omastx cluster id (for logging / correlation, never sent upstream).
	ID() uuid.UUID
	// Name is the omastx cluster name.
	Name() string
	// Clientset is a read-only client-go clientset for the cluster's API server.
	Clientset() kubernetes.Interface
}

// Artifact is a versioned thing discovered inside a cluster (SPEC §2.5 artifacts).
type Artifact struct {
	Kind      string // "image" | "helm" | ...
	Namespace string
	OwnerKind string // Deployment | StatefulSet | DaemonSet | CronJob | Pod
	OwnerName string
	// Identity is the normalized, registry-qualified name used for resolution and
	// dedup, e.g. "docker.io/library/nginx" or a Helm chart name.
	Identity string
	// Installed is the version/tag as found in the cluster.
	Installed string
	// SourceMeta carries kind-specific context (container name, registry host, ...).
	SourceMeta map[string]any
}

// Latest is a resolver's answer to "what is the newest version of this artifact?".
// Version is empty when nothing comparable was found (still recorded, never guessed).
type Latest struct {
	Version string // best candidate within the installed tag's channel
	// Candidates are the comparable versions considered, newest first.
	Candidates []string
	// ReleasesBehind counts published versions strictly between installed and latest
	// when the resolver can enumerate them; nil otherwise.
	ReleasesBehind *int
	// Deprecated is set when the resolver knows the installed line is discontinued.
	Deprecated bool
	// Confidence is the upstream match certainty (0–1). Low values mean the resolver
	// used a heuristic (e.g. Artifact Hub search) rather than an explicit repo URL.
	Confidence float32
	// RepoURL is the Helm chart repository URL used for resolution (helm only).
	RepoURL string
	ResolvedAt time.Time
}

// ArtifactProvider discovers versioned things inside a cluster.
// v1 ships: ImageProvider, HelmProvider. Future: OperatorProvider, NodeProvider...
type ArtifactProvider interface {
	Kind() string // "image" | "helm" | ...
	Discover(ctx context.Context, c ClusterClient) ([]Artifact, error)
}

// VersionResolver answers "what is the latest version of this artifact?"
// v1 ships: OCIRegistryResolver, HelmRepoResolver, ArtifactHubResolver.
type VersionResolver interface {
	CanResolve(a Artifact) bool
	Resolve(ctx context.Context, a Artifact) (Latest, error) // cached, rate-limited
}
