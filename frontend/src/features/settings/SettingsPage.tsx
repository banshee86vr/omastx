import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type AppSettings,
  type PutRegistryAuthInput,
} from "../../lib/api.ts";
import { Button, Field, useToast } from "../../ui/index.ts";
import { type Theme, getStoredTheme, toggleTheme } from "../../lib/theme.ts";
import styles from "./SettingsPage.module.css";

const settingsQueryKey = ["settings"];

export function SettingsPage() {
  const queryClient = useQueryClient();
  const { toast, toastError } = useToast();
  const { data, isLoading } = useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => api.getSettings(),
  });

  const [theme, setTheme] = useState<Theme>(() => getStoredTheme());
  const [ttl, setTtl] = useState<AppSettings | null>(null);
  const [credKind, setCredKind] = useState<"image" | "helm">("image");
  const [credTarget, setCredTarget] = useState("");
  const [credUser, setCredUser] = useState("");
  const [credPass, setCredPass] = useState("");

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

  function onThemeToggle() {
    setTheme(toggleTheme(theme));
  }

  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Settings</h1>
        <p className={styles.subline}>Theme, resolver cache TTLs, and global registry credentials.</p>
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
    </div>
  );
}
