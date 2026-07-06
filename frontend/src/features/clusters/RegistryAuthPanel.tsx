import { useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type PutRegistryAuthInput, type RegistryAuthEntry } from "../../lib/api.ts";
import { Button, useToast } from "../../ui/index.ts";
import { clusterSecretsQuery } from "./clusters.ts";
import styles from "./RegistryAuthPanel.module.css";

interface Props {
  clusterId: string;
  helmDiscoveryOK: boolean;
}

function usesPullSecret(kind: "image" | "helm", method: "pull_secret" | "basic") {
  return kind === "image" || method === "pull_secret";
}

export function RegistryAuthPanel({ clusterId, helmDiscoveryOK }: Props) {
  const queryClient = useQueryClient();
  const { toast, toastError } = useToast();
  const { data } = useQuery({
    queryKey: ["registry-auth", clusterId],
    queryFn: () => api.listRegistryAuth(clusterId),
  });

  const [kind, setKind] = useState<"image" | "helm">("image");
  const [method, setMethod] = useState<"pull_secret" | "basic">("pull_secret");
  const [target, setTarget] = useState("");
  const [secretName, setSecretName] = useState("");
  const [secretNamespace, setSecretNamespace] = useState("");
  const [secretUsernameKey, setSecretUsernameKey] = useState("");
  const [secretPasswordKey, setSecretPasswordKey] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const showPullSecretFields = usesPullSecret(kind, method);
  const {
    data: clusterSecrets,
    isLoading: clusterSecretsLoading,
    isError: clusterSecretsError,
    error: clusterSecretsLoadError,
  } = useQuery({
    ...clusterSecretsQuery(clusterId),
    enabled: showPullSecretFields,
  });

  const namespaces = useMemo(
    () => [...new Set(clusterSecrets?.map((s) => s.namespace) ?? [])].sort(),
    [clusterSecrets],
  );

  const secretsInNamespace = useMemo(
    () => clusterSecrets?.filter((s) => s.namespace === secretNamespace) ?? [],
    [clusterSecrets, secretNamespace],
  );

  const selectedSecret = useMemo(
    () => secretsInNamespace.find((s) => s.name === secretName),
    [secretsInNamespace, secretName],
  );

  const save = useMutation({
    mutationFn: (input: PutRegistryAuthInput) => api.putRegistryAuth(clusterId, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["registry-auth", clusterId] });
      toast("Credentials saved", "Re-scan the cluster to resolve private artifacts.");
      setTarget("");
      setSecretName("");
      setSecretNamespace("");
      setSecretUsernameKey("");
      setSecretPasswordKey("");
      setUsername("");
      setPassword("");
    },
    onError: (err) => {
      toastError("Couldn't save credentials", err instanceof Error ? err.message : "Try again.");
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    const input: PutRegistryAuthInput = {
      target: target.trim(),
      kind,
      method: kind === "helm" && method === "basic" ? "basic" : "pull_secret",
    };
    if (input.method === "pull_secret") {
      input.secret_name = secretName.trim();
      input.secret_namespace = secretNamespace.trim();
      input.secret_username_key = secretUsernameKey.trim();
      input.secret_password_key = secretPasswordKey.trim();
    } else {
      input.username = username.trim();
      input.password = password;
    }
    void save.mutateAsync(input);
  }

  function onNamespaceChange(value: string) {
    setSecretNamespace(value);
    setSecretName("");
    setSecretUsernameKey("");
    setSecretPasswordKey("");
  }

  function onSecretChange(value: string) {
    setSecretName(value);
    const secret = secretsInNamespace.find((s) => s.name === value);
    const keys = secret?.keys ?? [];
    const userDefault = keys.find((k) => k === "username") ?? keys[0] ?? "";
    const passDefault = keys.find((k) => k === "password") ?? keys.find((k) => k !== userDefault) ?? keys[0] ?? "";
    setSecretUsernameKey(userDefault);
    setSecretPasswordKey(passDefault);
  }

  const secretsLoadDetail =
    clusterSecretsLoadError instanceof Error ? clusterSecretsLoadError.message : undefined;

  return (
    <section className={styles.section} aria-label="Registry credentials">
      <h2 className={styles.title}>Registry credentials</h2>
      <p className={styles.help}>
        Private images use the workload&apos;s imagePullSecrets when present. If a registry still
        can&apos;t be reached, pick username and password keys from a secret in any namespace your
        kubeconfig can access. Private Helm repos need username and password, or secret key
        references.
      </p>
      {!helmDiscoveryOK && (
        <p className={styles.warn} role="status">
          Helm release discovery is disabled for this cluster because cluster-wide secrets list
          failed the connect-time check. You can still reference secrets here if your kubeconfig
          can read them in specific namespaces.
        </p>
      )}
      {data && data.length > 0 && (
        <ul className={styles.list}>
          {data.map((row: RegistryAuthEntry) => (
            <li key={`${row.kind}:${row.target}`}>
              <span className={styles.mono}>{row.target}</span>
              <span className={styles.muted}>
                {row.kind} · {row.method}
                {row.secret_name
                  ? ` · ${row.secret_namespace ?? "?"}/${row.secret_name}${row.secret_username_key || row.secret_password_key ? `#${row.secret_username_key ?? "?"} + ${row.secret_password_key ?? "?"}` : ""}`
                  : ""}
                {row.has_password ? " · basic auth" : ""}
              </span>
            </li>
          ))}
        </ul>
      )}
      <form className={styles.form} onSubmit={submit}>
        <label className={styles.label}>
          Kind
          <select className={styles.input} value={kind} onChange={(e) => setKind(e.target.value as "image" | "helm")}>
            <option value="image">Container registry</option>
            <option value="helm">Helm chart repo</option>
          </select>
        </label>
        <label className={styles.label}>
          {kind === "image" ? "Registry host" : "Chart repo URL"}
          <input
            className={styles.input}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder={kind === "image" ? "ghcr.io" : "https://charts.example.com"}
            required
          />
        </label>
        {kind === "helm" && (
          <label className={styles.label}>
            Auth method
            <select
              className={styles.input}
              value={method}
              onChange={(e) => setMethod(e.target.value as "pull_secret" | "basic")}
            >
              <option value="basic">Username and password</option>
              <option value="pull_secret">Kubernetes secret key</option>
            </select>
          </label>
        )}
        {showPullSecretFields && (
          <>
            {clusterSecretsLoading ? (
              <p className={styles.muted}>Loading secrets from the cluster…</p>
            ) : clusterSecretsError ? (
              <p className={styles.warn} role="status">
                Couldn&apos;t load secrets from the cluster
                {secretsLoadDetail ? `: ${secretsLoadDetail}` : ""}. Check that your kubeconfig
                context can get/list secrets in at least one namespace, then reload the page.
              </p>
            ) : clusterSecrets && clusterSecrets.length === 0 ? (
              <p className={styles.warn} role="status">
                No readable secrets were found in namespaces your kubeconfig can access.
              </p>
            ) : (
              <>
                <label className={styles.label}>
                  Namespace
                  <select
                    className={styles.input}
                    value={secretNamespace}
                    onChange={(e) => onNamespaceChange(e.target.value)}
                    required
                  >
                    <option value="">Choose a namespace</option>
                    {namespaces.map((ns) => (
                      <option key={ns} value={ns}>
                        {ns}
                      </option>
                    ))}
                  </select>
                </label>
                <label className={styles.label}>
                  Secret
                  <select
                    className={styles.input}
                    value={secretName}
                    onChange={(e) => onSecretChange(e.target.value)}
                    required
                    disabled={!secretNamespace}
                  >
                    <option value="">
                      {secretNamespace ? "Choose a secret" : "Pick a namespace first"}
                    </option>
                    {secretsInNamespace.map((s) => (
                      <option key={s.name} value={s.name}>
                        {s.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className={styles.label}>
                  Username key
                  <select
                    className={styles.input}
                    value={secretUsernameKey}
                    onChange={(e) => setSecretUsernameKey(e.target.value)}
                    required
                    disabled={!secretName}
                  >
                    <option value="">
                      {secretName ? "Choose username key" : "Pick a secret first"}
                    </option>
                    {selectedSecret?.keys.map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </select>
                </label>
                <label className={styles.label}>
                  Password key
                  <select
                    className={styles.input}
                    value={secretPasswordKey}
                    onChange={(e) => setSecretPasswordKey(e.target.value)}
                    required
                    disabled={!secretName}
                  >
                    <option value="">
                      {secretName ? "Choose password key" : "Pick a secret first"}
                    </option>
                    {selectedSecret?.keys.map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </select>
                </label>
              </>
            )}
          </>
        )}
        {kind === "helm" && method === "basic" && (
          <>
            <label className={styles.label}>
              Username
              <input className={styles.input} value={username} onChange={(e) => setUsername(e.target.value)} />
            </label>
            <label className={styles.label}>
              Password
              <input
                className={styles.input}
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </label>
          </>
        )}
        <Button
          type="submit"
          disabled={
            save.isPending ||
            (showPullSecretFields &&
              (!clusterSecrets ||
                clusterSecrets.length === 0 ||
                !secretNamespace ||
                !secretName ||
                !secretUsernameKey ||
                !secretPasswordKey))
          }
        >
          {save.isPending ? "Saving…" : "Save credentials"}
        </Button>
      </form>
    </section>
  );
}
