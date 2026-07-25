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
