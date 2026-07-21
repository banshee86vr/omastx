import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type AppSettings,
  type APITokenScope,
  type PutRegistryAuthInput,
} from "../../lib/api.ts";
import { Button, Field, useToast } from "../../ui/index.ts";
import { type Theme, getStoredTheme, toggleTheme } from "../../lib/theme.ts";
import styles from "./SettingsPage.module.css";

const settingsQueryKey = ["settings"];
const tokensQueryKey = ["api-tokens"];

export function SettingsPage() {
  const queryClient = useQueryClient();
  const { toast, toastError } = useToast();
  const { data, isLoading } = useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => api.getSettings(),
  });
  const { data: tokens = [] } = useQuery({
    queryKey: tokensQueryKey,
    queryFn: () => api.listAPITokens(),
  });

  const [theme, setTheme] = useState<Theme>(() => getStoredTheme());
  const [ttl, setTtl] = useState<AppSettings | null>(null);
  const [credKind, setCredKind] = useState<"image" | "helm">("image");
  const [credTarget, setCredTarget] = useState("");
  const [credUser, setCredUser] = useState("");
  const [credPass, setCredPass] = useState("");
  const [tokenName, setTokenName] = useState("");
  const [tokenScopes, setTokenScopes] = useState<APITokenScope[]>(["read", "scan"]);
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  const settings = ttl ?? data?.settings;

  const saveTtl = useMutation({
    mutationFn: (s: AppSettings) => api.putSettings(s),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: settingsQueryKey });
      toast("Cache TTLs updated", "New resolver cache windows apply on the next upstream fetch.");
    },
    onError: (err) => toastError("Couldn't save TTLs", err instanceof Error ? err.message : "Try again."),
  });

  const saveCred = useMutation({
    mutationFn: (input: PutRegistryAuthInput) => api.putGlobalRegistryAuth(input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: settingsQueryKey });
      toast("Global credentials saved", "They apply when no per-cluster credential matches.");
      setCredTarget("");
      setCredUser("");
      setCredPass("");
    },
    onError: (err) =>
      toastError("Couldn't save credentials", err instanceof Error ? err.message : "Try again."),
  });

  function onTtlSubmit(e: FormEvent) {
    e.preventDefault();
    if (!settings) return;
    void saveTtl.mutateAsync(settings);
  }

  function onCredSubmit(e: FormEvent) {
    e.preventDefault();
    void saveCred.mutateAsync({
      target: credTarget.trim(),
      kind: credKind,
      method: "basic",
      username: credUser.trim(),
      password: credPass,
    });
  }

  const createToken = useMutation({
    mutationFn: () =>
      api.createAPIToken({
        name: tokenName.trim(),
        scopes: tokenScopes,
      }),
    onSuccess: async (resp) => {
      await queryClient.invalidateQueries({ queryKey: tokensQueryKey });
      setCreatedToken(resp.token);
      setTokenName("");
      toast("API token created", "Copy it now — it will not be shown again.");
    },
    onError: (err) =>
      toastError("Couldn't create token", err instanceof Error ? err.message : "Try again."),
  });

  const revokeToken = useMutation({
    mutationFn: (id: string) => api.revokeAPIToken(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: tokensQueryKey });
      toast("Token revoked", "Scripts using that token will stop working immediately.");
    },
    onError: (err) =>
      toastError("Couldn't revoke token", err instanceof Error ? err.message : "Try again."),
  });

  function onThemeToggle() {
    setTheme(toggleTheme(theme));
  }

  function onTokenSubmit(e: FormEvent) {
    e.preventDefault();
    if (!tokenName.trim() || tokenScopes.length === 0) return;
    void createToken.mutateAsync();
  }

  function toggleScope(scope: APITokenScope) {
    setTokenScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope],
    );
  }

  async function copyCreatedToken() {
    if (!createdToken) return;
    try {
      await navigator.clipboard.writeText(createdToken);
      toast("Copied", "Token copied to the clipboard.");
    } catch {
      toastError("Couldn't copy", "Select the token and copy it manually.");
    }
  }

  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Settings</h1>
        <p className={styles.subline}>
          Theme, resolver cache TTLs, global registry credentials, and machine API tokens.
        </p>
      </header>

      {isLoading && <p className={styles.muted}>Loading settings…</p>}

      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>Theme</h2>
        <p className={styles.muted}>
          Night is the default operator view. Daylight chart uses the same tokens with adjusted contrast.
        </p>
        <Button variant="primary" onClick={onThemeToggle}>
          Switch to {theme === "night" ? "daylight" : "night"} chart
        </Button>
      </section>

      {settings && (
        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Resolver cache TTLs</h2>
          <form className={styles.form} onSubmit={onTtlSubmit}>
            <Field
              label="OCI registry TTL (hours)"
              type="number"
              min={1}
              max={168}
              value={settings.oci_ttl_hours}
              onChange={(e) =>
                setTtl({ ...settings, oci_ttl_hours: Number(e.target.value) })
              }
            />
            <Field
              label="Helm repo TTL (hours)"
              type="number"
              min={1}
              max={168}
              value={settings.helmrepo_ttl_hours}
              onChange={(e) =>
                setTtl({ ...settings, helmrepo_ttl_hours: Number(e.target.value) })
              }
            />
            <Field
              label="Artifact Hub TTL (hours)"
              type="number"
              min={1}
              max={168}
              value={settings.artifacthub_ttl_hours}
              onChange={(e) =>
                setTtl({ ...settings, artifacthub_ttl_hours: Number(e.target.value) })
              }
            />
            <Button type="submit" variant="primary" disabled={saveTtl.isPending}>
              Save TTLs
            </Button>
          </form>
        </section>
      )}

      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>Global registry credentials</h2>
        <p className={styles.muted}>
          Fallback when no per-cluster credential matches a private registry or Helm repo.
        </p>
        <ul className={styles.credList}>
          {data?.global_registry_auth.map((c) => (
            <li key={`${c.kind}:${c.target}`}>
              <code>{c.target}</code> ({c.kind}) {c.has_password ? "· saved" : ""}
            </li>
          ))}
        </ul>
        <form className={styles.form} onSubmit={onCredSubmit}>
          <label className={styles.label}>
            Kind
            <select value={credKind} onChange={(e) => setCredKind(e.target.value as "image" | "helm")}>
              <option value="image">image</option>
              <option value="helm">helm</option>
            </select>
          </label>
          <Field
            label="Target (registry host or chart repo URL)"
            value={credTarget}
            onChange={(e) => setCredTarget(e.target.value)}
            required
          />
          <Field label="Username" value={credUser} onChange={(e) => setCredUser(e.target.value)} />
          <Field
            label="Password"
            type="password"
            value={credPass}
            onChange={(e) => setCredPass(e.target.value)}
          />
          <Button type="submit" variant="primary" disabled={saveCred.isPending}>
            Save global credentials
          </Button>
        </form>
      </section>

      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>API tokens</h2>
        <p className={styles.muted}>
          Bearer tokens for scripts and AI agents. Scopes: <code>read</code> (fleet, artifacts,
          scans) and <code>scan</code> (start scans). Admin actions stay session-only.
        </p>
        {createdToken && (
          <div className={styles.tokenReveal} role="status">
            <p className={styles.muted}>Copy this token now — it will not be shown again.</p>
            <code className={styles.tokenValue}>{createdToken}</code>
            <div className={styles.tokenActions}>
              <Button type="button" variant="primary" onClick={() => void copyCreatedToken()}>
                Copy token
              </Button>
              <Button type="button" variant="quiet" onClick={() => setCreatedToken(null)}>
                Dismiss
              </Button>
            </div>
          </div>
        )}
        <ul className={styles.credList}>
          {tokens.map((tok) => (
            <li key={tok.id} className={styles.userRow}>
              <span>
                <strong>{tok.name}</strong> · <code>{tok.prefix}…</code> · {tok.scopes.join(", ")}
              </span>
              <Button
                type="button"
                variant="danger"
                onClick={() => void revokeToken.mutateAsync(tok.id)}
                disabled={revokeToken.isPending}
              >
                Revoke
              </Button>
            </li>
          ))}
          {tokens.length === 0 && <li className={styles.muted}>No active tokens yet.</li>}
        </ul>
        <form className={styles.form} onSubmit={onTokenSubmit}>
          <Field
            label="Token name"
            value={tokenName}
            onChange={(e) => setTokenName(e.target.value)}
            required
            maxLength={128}
          />
          <fieldset className={styles.scopeSet}>
            <legend>Scopes</legend>
            <label className={styles.scopeLabel}>
              <input
                type="checkbox"
                checked={tokenScopes.includes("read")}
                onChange={() => toggleScope("read")}
              />
              read
            </label>
            <label className={styles.scopeLabel}>
              <input
                type="checkbox"
                checked={tokenScopes.includes("scan")}
                onChange={() => toggleScope("scan")}
              />
              scan
            </label>
          </fieldset>
          <Button
            type="submit"
            variant="primary"
            disabled={createToken.isPending || tokenScopes.length === 0}
          >
            Create token
          </Button>
        </form>
      </section>
    </div>
  );
}
