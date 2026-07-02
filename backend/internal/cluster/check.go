package cluster

import (
	"context"
	"fmt"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// Permission is one row of the RBAC self-check matrix shown in the UI.
type Permission struct {
	Group    string `json:"group"`
	Resource string `json:"resource"`
	Verb     string `json:"verb"`
	Allowed  bool   `json:"allowed"`
}

// RBACReport is persisted as clusters.rbac_report (jsonb).
type RBACReport struct {
	Permissions []Permission `json:"permissions"`
	// ImagesOK: every non-secret permission granted (workload discovery works).
	ImagesOK bool `json:"images_ok"`
	// HelmOK: secrets get/list granted (Helm 3 release data readable).
	HelmOK    bool      `json:"helm_ok"`
	CheckedAt time.Time `json:"checked_at"`
}

// CheckResult is what the connect flow shows before anything is stored.
type CheckResult struct {
	Server    string     `json:"server"`
	Reachable bool       `json:"reachable"`
	Version   string     `json:"version,omitempty"`
	Error     string     `json:"error,omitempty"`
	RBAC      RBACReport `json:"rbac"`
}

// checkedResources is EXACTLY the read-only set from SPEC §2.6 — get/list on
// workload kinds plus secrets (Helm 3 releases). Never add write verbs.
var checkedResources = []struct {
	group    string
	resource string
}{
	{"", "pods"},
	{"", "namespaces"},
	{"apps", "deployments"},
	{"apps", "statefulsets"},
	{"apps", "daemonsets"},
	{"batch", "cronjobs"},
	{"", "secrets"},
}

var checkedVerbs = []string{"get", "list"}

// Connector abstracts real cluster access so API handlers can be tested
// without a live cluster.
type Connector interface {
	// Check tests connectivity and runs the RBAC self-check for one context.
	Check(ctx context.Context, kubeconfig []byte, contextName string) (CheckResult, error)
}

// KubeConnector is the production Connector backed by client-go.
type KubeConnector struct {
	// Timeout bounds the whole check (connection + reviews).
	Timeout time.Duration
}

func (k *KubeConnector) Check(ctx context.Context, kubeconfig []byte, contextName string) (CheckResult, error) {
	restCfg, err := RESTConfig(kubeconfig, contextName)
	if err != nil {
		return CheckResult{}, err
	}
	timeout := k.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	restCfg.Timeout = timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := CheckResult{Server: restCfg.Host}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		result.Error = fmt.Sprintf("build client: %v", err)
		return result, nil
	}
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Reachable = true
	result.Version = version.GitVersion

	report, err := RunRBACSelfCheck(ctx, clientset.AuthorizationV1())
	if err != nil {
		result.Error = fmt.Sprintf("RBAC self-check failed: %v", err)
		return result, nil
	}
	result.RBAC = report
	return result, nil
}

// RunRBACSelfCheck issues SelfSubjectAccessReviews for exactly the SPEC §2.6
// read-only set and derives the images/helm capability flags.
func RunRBACSelfCheck(ctx context.Context, client authorizationclient.AuthorizationV1Interface) (RBACReport, error) {
	report := RBACReport{CheckedAt: time.Now().UTC(), ImagesOK: true, HelmOK: true}
	for _, res := range checkedResources {
		for _, verb := range checkedVerbs {
			review := &authorizationv1.SelfSubjectAccessReview{
				Spec: authorizationv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Group:    res.group,
						Resource: res.resource,
						Verb:     verb,
					},
				},
			}
			resp, err := client.SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
			if err != nil {
				return RBACReport{}, fmt.Errorf("review %s %s: %w", verb, res.resource, err)
			}
			allowed := resp.Status.Allowed
			report.Permissions = append(report.Permissions, Permission{
				Group:    res.group,
				Resource: res.resource,
				Verb:     verb,
				Allowed:  allowed,
			})
			if !allowed {
				if res.resource == "secrets" {
					report.HelmOK = false
				} else {
					report.ImagesOK = false
				}
			}
		}
	}
	return report, nil
}
