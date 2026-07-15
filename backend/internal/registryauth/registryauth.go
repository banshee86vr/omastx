// Package registryauth resolves registry and Helm repo credentials during scans.
// For images it prefers imagePullSecrets declared on the workload pod spec, then
// cluster-configured pull-secret references. For Helm it uses stored basic auth.
// Secret contents are read in memory only and never persisted (SPEC §2.6).
package registryauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
)

// ErrAuthRequired is returned when upstream requires credentials we don't have.
var ErrAuthRequired = errors.New("registry authentication required")

// AuthRequiredError carries context for the UI to prompt for credentials.
type AuthRequiredError struct {
	Kind   string // "image" | "helm"
	Target string // registry host or repo URL
	Detail string
}

func (e *AuthRequiredError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("%s authentication required for %s", e.Kind, e.Target)
}

func (e *AuthRequiredError) Unwrap() error { return ErrAuthRequired }

// IsAuthRequired reports whether err (or its chain) signals missing credentials.
func IsAuthRequired(err error) (*AuthRequiredError, bool) {
	var ae *AuthRequiredError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// IsUnauthorizedHTTP reports common registry/chart-repo auth failure responses.
func IsUnauthorizedHTTP(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

type ctxKey struct{}

// WithProvider attaches a scan-scoped credential provider to ctx.
func WithProvider(ctx context.Context, p *Provider) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the credential provider installed for this scan, if any.
func FromContext(ctx context.Context) *Provider {
	p, _ := ctx.Value(ctxKey{}).(*Provider)
	return p
}

// ConfiguredAuth is a cluster-stored credential reference (not secret contents).
type ConfiguredAuth struct {
	Target          string
	Kind            string // image | helm
	Method          string // pull_secret | basic
	SecretNamespace   string
	SecretName        string
	SecretUsernameKey string
	SecretPasswordKey string
	Username          string // decrypted in memory only
	Password        string
}

// Store loads cluster registry auth configuration.
type Store interface {
	ListRegistryAuth(ctx context.Context, clusterID uuid.UUID) ([]ConfiguredAuth, error)
	ListGlobalRegistryAuth(ctx context.Context) ([]ConfiguredAuth, error)
}

// Provider resolves credentials for resolvers during a cluster scan.
type Provider struct {
	client    kubernetes.Interface
	clusterID uuid.UUID
	store     Store
	masterKey []byte

	mu    sync.Mutex
	cache map[string]authn.Authenticator // host -> auth
	helm  map[string]basicCreds          // repo URL -> basic
}

type basicCreds struct {
	user, pass string
}

func NewProvider(client kubernetes.Interface, clusterID uuid.UUID, store Store, masterKey []byte) *Provider {
	return &Provider{
		client:    client,
		clusterID: clusterID,
		store:     store,
		masterKey: masterKey,
		cache:     map[string]authn.Authenticator{},
		helm:      map[string]basicCreds{},
	}
}

// AuthForImage returns an authenticator for the artifact's registry host.
// Order: workload imagePullSecrets (from the pod spec), then cluster pull_secret ref.
func (p *Provider) AuthForImage(ctx context.Context, a core.Artifact) (authn.Authenticator, error) {
	if p == nil || p.client == nil {
		return nil, nil
	}
	host := registryHost(a.Identity)
	if host == "" {
		return nil, nil
	}

	p.mu.Lock()
	if auth, ok := p.cache[host]; ok {
		p.mu.Unlock()
		return auth, nil
	}
	p.mu.Unlock()

	names := pullSecretNames(a)
	if len(names) > 0 {
		if auth, err := p.authFromSecrets(ctx, a.Namespace, names, "", "", host); err == nil && auth != nil {
			p.rememberHost(host, auth)
			return auth, nil
		}
	}
	if cfg, ok := p.configuredPullSecret(ctx, host, "image"); ok {
		ns := cfg.SecretNamespace
		if ns == "" {
			ns = a.Namespace
		}
		if auth, err := p.authFromSecrets(ctx, ns, []string{cfg.SecretName}, cfg.SecretUsernameKey, cfg.SecretPasswordKey, host); err == nil && auth != nil {
			p.rememberHost(host, auth)
			return auth, nil
		}
	}
	if auth := p.globalBasicAuth(ctx, host, "image"); auth != nil {
		p.rememberHost(host, auth)
		return auth, nil
	}

	return nil, nil
}

// BasicForHelmRepo returns username/password for a private chart repository.
func (p *Provider) BasicForHelmRepo(ctx context.Context, repoURL string) (user, pass string, ok bool) {
	if p == nil {
		return "", "", false
	}
	key := normalizeRepoURL(repoURL)
	p.mu.Lock()
	if c, hit := p.helm[key]; hit {
		p.mu.Unlock()
		return c.user, c.pass, c.user != "" || c.pass != ""
	}
	p.mu.Unlock()

	if p.store == nil {
		return "", "", false
	}
	rows, err := p.store.ListRegistryAuth(ctx, p.clusterID)
	if err != nil {
		return "", "", false
	}
	for _, row := range rows {
		if row.Kind != "helm" {
			continue
		}
		rowKey := normalizeRepoURL(row.Target)
		if rowKey != key {
			continue
		}
		switch row.Method {
		case "basic":
			p.mu.Lock()
			p.helm[key] = basicCreds{user: row.Username, pass: row.Password}
			p.mu.Unlock()
			return row.Username, row.Password, row.Username != "" || row.Password != ""
		case "pull_secret":
			auth, err := p.authFromSecrets(ctx, row.SecretNamespace, []string{row.SecretName}, row.SecretUsernameKey, row.SecretPasswordKey, "")
			if err != nil || auth == nil {
				continue
			}
			if ac, err := auth.Authorization(); err == nil {
				p.mu.Lock()
				p.helm[key] = basicCreds{user: ac.Username, pass: ac.Password}
				p.mu.Unlock()
				return ac.Username, ac.Password, ac.Username != "" || ac.Password != ""
			}
		}
	}
	if user, pass, ok := p.globalBasicForHelm(ctx, key); ok {
		return user, pass, true
	}
	return "", "", false
}

// HelmRepoTargets returns distinct Helm chart repository URLs configured for the cluster.
func (p *Provider) HelmRepoTargets(ctx context.Context) []string {
	if p == nil || p.store == nil {
		return nil
	}
	rows, err := p.store.ListRegistryAuth(ctx, p.clusterID)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, row := range rows {
		if row.Kind != "helm" {
			continue
		}
		key := normalizeRepoURL(row.Target)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (p *Provider) configuredPullSecret(ctx context.Context, target, kind string) (ConfiguredAuth, bool) {
	if p.store == nil {
		return ConfiguredAuth{}, false
	}
	rows, err := p.store.ListRegistryAuth(ctx, p.clusterID)
	if err != nil {
		return ConfiguredAuth{}, false
	}
	for _, row := range rows {
		if row.Kind == kind && row.Method == "pull_secret" && strings.EqualFold(row.Target, target) {
			return row, true
		}
	}
	return ConfiguredAuth{}, false
}

func (p *Provider) globalBasicAuth(ctx context.Context, target, kind string) authn.Authenticator {
	if p.store == nil {
		return nil
	}
	rows, err := p.store.ListGlobalRegistryAuth(ctx)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		if row.Kind != kind || row.Method != "basic" {
			continue
		}
		if kind == "image" && !strings.EqualFold(row.Target, target) {
			continue
		}
		if row.Username != "" || row.Password != "" {
			return &authn.Basic{Username: row.Username, Password: row.Password}
		}
	}
	return nil
}

func (p *Provider) globalBasicForHelm(ctx context.Context, repoURL string) (user, pass string, ok bool) {
	if p.store == nil {
		return "", "", false
	}
	rows, err := p.store.ListGlobalRegistryAuth(ctx)
	if err != nil {
		return "", "", false
	}
	key := normalizeRepoURL(repoURL)
	for _, row := range rows {
		if row.Kind != "helm" || row.Method != "basic" {
			continue
		}
		if normalizeRepoURL(row.Target) != key {
			continue
		}
		return row.Username, row.Password, row.Username != "" || row.Password != ""
	}
	return "", "", false
}

func (p *Provider) authFromSecrets(ctx context.Context, namespace string, names []string, userKey, passKey, host string) (authn.Authenticator, error) {
	for _, name := range names {
		if name == "" {
			continue
		}
		sec, err := p.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if auth := authFromSecretKeys(sec, userKey, passKey, host); auth != nil {
			return auth, nil
		}
	}
	return nil, fmt.Errorf("no matching credentials in secrets %v", names)
}

func authFromSecretKeys(sec *corev1.Secret, userKey, passKey, host string) authn.Authenticator {
	if passKey != "" {
		if userKey != "" {
			pass, pok := sec.Data[passKey]
			if pok && len(pass) > 0 {
				user := "token"
				if u, uok := sec.Data[userKey]; uok && len(u) > 0 {
					user = string(u)
				}
				// When both keys are set, prefer explicit username/password unless the
				// password field holds embedded dockerconfig JSON (common in .dockerconfigjson secrets).
				if strings.HasPrefix(strings.TrimSpace(string(pass)), "{") {
					if auth := authFromCredentialBytes(pass, host); auth != nil {
						return auth
					}
				}
				return &authn.Basic{Username: user, Password: string(pass)}
			}
		}
		if raw, ok := sec.Data[passKey]; ok {
			if auth := authFromCredentialBytes(raw, host); auth != nil {
				return auth
			}
		}
	}
	if userKey == "" && passKey == "" {
		return authFromSecretData(sec.Data, host)
	}
	return nil
}

func authFromCredentialBytes(raw []byte, host string) authn.Authenticator {
	if auth := authFromDockerConfigJSON(raw, host); auth != nil {
		return auth
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "" && !strings.HasPrefix(trimmed, "{") {
		return &authn.Basic{Username: "token", Password: trimmed}
	}
	return nil
}

func (p *Provider) rememberHost(host string, auth authn.Authenticator) {
	p.mu.Lock()
	p.cache[host] = auth
	p.mu.Unlock()
}

func pullSecretNames(a core.Artifact) []string {
	if a.SourceMeta == nil {
		return nil
	}
	raw, ok := a.SourceMeta["image_pull_secrets"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func authFromSecretData(data map[string][]byte, host string) authn.Authenticator {
	if raw, ok := data[corev1.DockerConfigJsonKey]; ok {
		return authFromDockerConfigJSON(raw, host)
	}
	if raw, ok := data[corev1.DockerConfigKey]; ok {
		return authFromDockerConfigJSON(raw, host)
	}
	return nil
}

// RegistryHostsFromDockerConfigJSON returns normalized registry hostnames parsed from
// dockerconfigjson/dockercfg auths keys. Credential values are never returned.
func RegistryHostsFromDockerConfigJSON(raw []byte) []string {
	var cfg dockerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var hosts []string
	for key := range cfg.Auths {
		host := normalizeRegistryHost(key)
		if host == "" {
			continue
		}
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func normalizeRegistryHost(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimSuffix(key, "/")
	key = strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	if i := strings.IndexByte(key, '/'); i >= 0 {
		key = key[:i]
	}
	switch key {
	case "", "https:", "http:":
		return ""
	case "index.docker.io":
		return "docker.io"
	default:
		return key
	}
}

type dockerConfig struct {
	Auths map[string]dockerAuth `json:"auths"`
}

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

func authFromDockerConfigJSON(raw []byte, host string) authn.Authenticator {
	var cfg dockerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	if host != "" {
		if a := pickAuth(cfg.Auths, host); a != nil {
			return a
		}
	}
	for _, a := range cfg.Auths {
		if auth := dockerAuthToAuthenticator(a); auth != nil {
			return auth
		}
	}
	return nil
}

func pickAuth(auths map[string]dockerAuth, host string) authn.Authenticator {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	candidates := []string{
		host,
		"https://" + host + "/v1/",
		"https://" + host + "/v2/",
		"https://" + host,
		host + "/v1/",
		host + "/v2/",
	}
	if host == "docker.io" || host == "index.docker.io" {
		candidates = append(candidates, "https://index.docker.io/v1/")
	}
	for _, key := range candidates {
		if a, ok := auths[key]; ok {
			if auth := dockerAuthToAuthenticator(a); auth != nil {
				return auth
			}
		}
	}
	return nil
}

func dockerAuthToAuthenticator(a dockerAuth) authn.Authenticator {
	user, pass := a.Username, a.Password
	if user == "" && a.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(a.Auth)
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				user, pass = parts[0], parts[1]
			}
		}
	}
	if user == "" && pass == "" {
		return nil
	}
	return &authn.Basic{Username: user, Password: pass}
}

func registryHost(identity string) string {
	if i := strings.IndexByte(identity, '/'); i >= 0 {
		host := identity[:i]
		if strings.ContainsAny(host, ".:") || host == "localhost" {
			return host
		}
	}
	return "docker.io"
}

func normalizeRepoURL(u string) string {
	return strings.TrimSuffix(strings.TrimSpace(u), "/")
}

// DecryptConfigured decrypts stored basic-auth fields for a configured row.
func DecryptConfigured(masterKey []byte, usernameEnc, usernameNonce, passwordEnc, passwordNonce []byte) (user, pass string, err error) {
	if len(usernameEnc) > 0 {
		raw, err := crypto.Decrypt(masterKey, usernameEnc, usernameNonce)
		if err != nil {
			return "", "", err
		}
		user = string(raw)
	}
	if len(passwordEnc) > 0 {
		raw, err := crypto.Decrypt(masterKey, passwordEnc, passwordNonce)
		if err != nil {
			return "", "", err
		}
		pass = string(raw)
	}
	return user, pass, nil
}

// NewAuthRequired builds a typed auth error for resolvers.
func NewAuthRequired(kind, target, detail string) error {
	return &AuthRequiredError{Kind: kind, Target: target, Detail: detail}
}
