# Security Policy

## Supported versions

Security fixes are applied on the default branch (`main`) and included in the
next tagged release. Older tags are not backported unless noted in the release
notes.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report privately via one of:

- GitHub Security Advisories: **Security → Report a vulnerability** on this
  repository
- Email: bertelli.luca@proton.me

Include a short description, impact, and steps to reproduce if possible. You
should receive an acknowledgement within a few days.

## Deployment notes

- Never set `OMASTX_DEV=true` in production (it enables passwordless sign-in and
  a fixed development master key).
- Keep `OMASTX_MASTER_KEY`, GitHub OAuth secrets, registry credentials, and API
  tokens out of git; use Kubernetes Secrets / env injection only.
- Connect clusters with **read-only** kubeconfigs. Omastx is designed not to
  request write verbs against the cluster.
- Kubeconfig credentials must be **embedded in the file** (client certificate
  data or a bearer token; `kubectl config view --raw --flatten` inlines
  certificate files). Omastx refuses `exec` credential plugins,
  `auth-provider` blocks, and credentials given as file paths (`tokenFile`,
  `client-certificate`, `client-key`, `certificate-authority`): the backend
  resolves an uploaded kubeconfig itself, so those would run a command or read a
  file on the Omastx host rather than the uploader's machine. Cloud token plugins
  (EKS/GKE/AKS) are a future integration (SPEC §1.6), not a supported one.
- Outbound scan traffic (API servers, chart repositories, image registries) is
  checked at connect time against the resolved IP: link-local addresses — which
  is where the cloud instance metadata service lives — multicast, and
  unspecified addresses are refused, and so is loopback unless `OMASTX_DEV=true`.
  Private ranges stay reachable so on-prem clusters and internal registries keep
  working.
