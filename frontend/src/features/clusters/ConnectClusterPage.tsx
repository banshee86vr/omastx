import { useRef, useState, type DragEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  api,
  ApiError,
  type CheckResult,
  type ContextInfo,
  type Problem,
} from "../../lib/api.ts";
import { clustersQuery } from "./clusters.ts";
import { Button, Field, Tag, useToast } from "../../ui/index.ts";
import { PermissionMatrix } from "./PermissionMatrix.tsx";
import styles from "./ConnectClusterPage.module.css";

const DEFAULT_SCHEDULE = "0 */6 * * *";

interface Candidate {
  context: ContextInfo;
  name: string;
  schedule: string;
  check?: CheckResult;
  checking: boolean;
  problem?: Problem | undefined;
  connected?: boolean;
}

function toProblem(err: unknown): Problem {
  if (err instanceof ApiError) return err.problem;
  return {
    code: "unreachable",
    title: "Couldn't reach the server",
    detail: "The request failed. Check that the backend is running, then retry.",
  };
}

export function ConnectClusterPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const [kubeconfig, setKubeconfig] = useState("");
  const [fileName, setFileName] = useState<string | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [inspectProblem, setInspectProblem] = useState<Problem | null>(null);
  const [contexts, setContexts] = useState<ContextInfo[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [candidates, setCandidates] = useState<Candidate[] | null>(null);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  async function loadFile(file: File) {
    const text = await file.text();
    setKubeconfig(text);
    setFileName(file.name);
    await inspect(text);
  }

  async function inspect(config: string) {
    setInspectProblem(null);
    setContexts(null);
    setCandidates(null);
    if (!config.trim()) return;
    try {
      const found = await api.inspectKubeconfig(config);
      setContexts(found);
      setSelected(
        new Set(found.filter((c) => c.current && !c.unsupported).map((c) => c.name)),
      );
    } catch (err) {
      setInspectProblem(toProblem(err));
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragActive(false);
    const file = e.dataTransfer.files[0];
    if (file) void loadFile(file);
  }

  function toggleContext(name: string) {
    if (contexts?.find((c) => c.name === name)?.unsupported) return;
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
    setCandidates(null);
  }

  async function checkSelected() {
    if (!contexts) return;
    const picked = contexts.filter((c) => selected.has(c.name));
    const initial: Candidate[] = picked.map((context) => ({
      context,
      name: context.name,
      schedule: DEFAULT_SCHEDULE,
      checking: true,
    }));
    setCandidates(initial);
    setBusy(true);
    try {
      await Promise.all(
        picked.map(async (context, i) => {
          try {
            const check = await api.checkCluster(kubeconfig, context.name);
            setCandidates((prev) =>
              prev ? prev.map((c, j) => (j === i ? { ...c, check, checking: false } : c)) : prev,
            );
          } catch (err) {
            const problem = toProblem(err);
            setCandidates((prev) =>
              prev ? prev.map((c, j) => (j === i ? { ...c, problem, checking: false } : c)) : prev,
            );
          }
        }),
      );
    } finally {
      setBusy(false);
    }
  }

  function updateCandidate(i: number, patch: Partial<Candidate>) {
    setCandidates((prev) => (prev ? prev.map((c, j) => (j === i ? { ...c, ...patch } : c)) : prev));
  }

  const connectable = (candidates ?? []).filter(
    (c) => c.check?.reachable && c.check.rbac.images_ok && !c.connected,
  );

  async function connectAll() {
    if (!candidates) return;
    setBusy(true);
    let connected = 0;
    try {
      for (const [i, candidate] of candidates.entries()) {
        if (!candidate.check?.reachable || !candidate.check.rbac.images_ok || candidate.connected) {
          continue;
        }
        try {
          await api.createCluster({
            name: candidate.name.trim(),
            kubeconfig,
            context: candidate.context.name,
            schedule_cron: candidate.schedule.trim(),
          });
          connected += 1;
          updateCandidate(i, { connected: true, problem: undefined });
        } catch (err) {
          updateCandidate(i, { problem: toProblem(err) });
        }
      }
    } finally {
      setBusy(false);
    }
    if (connected > 0) {
      await queryClient.invalidateQueries({ queryKey: clustersQuery.queryKey });
      toast(
        connected === 1 ? "Cluster connected" : `${connected} clusters connected`,
        "Scans will run on the configured schedule. You can also scan on demand from the cluster page.",
      );
      if (candidates.every((c) => c.connected || c.problem)) {
        await navigate({ to: "/" });
      }
    }
  }

  return (
    <div className={styles.page}>
      <header>
        <h1>Connect a cluster</h1>
        <p className={styles.hint}>
          Omastx only needs a read-only kubeconfig: get/list on workloads, plus secrets to read
          Helm releases. It never writes to your clusters.
        </p>
      </header>

      <section className={styles.step} aria-label="Step 1: kubeconfig">
        <h2 className={styles.stepTitle}>
          <span className={styles.stepNumber}>1</span> Provide a kubeconfig
        </h2>
        <div
          className={[styles.dropzone, dragActive ? styles.dropzoneActive : ""].join(" ")}
          onDragOver={(e) => {
            e.preventDefault();
            setDragActive(true);
          }}
          onDragLeave={() => setDragActive(false)}
          onDrop={onDrop}
          onClick={() => fileInput.current?.click()}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") fileInput.current?.click();
          }}
          role="button"
          tabIndex={0}
          aria-label="Upload kubeconfig file"
        >
          {fileName ? (
            <>
              Loaded <span className={styles.fileName}>{fileName}</span> - drop another file to
              replace it
            </>
          ) : (
            <>Drop a kubeconfig file here, or click to choose one</>
          )}
        </div>
        <input
          ref={fileInput}
          type="file"
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) void loadFile(file);
            e.target.value = "";
          }}
        />
        <div className={styles.divider}>or paste it</div>
        <textarea
          className={styles.paste}
          placeholder="apiVersion: v1&#10;kind: Config&#10;..."
          value={kubeconfig}
          onChange={(e) => {
            setKubeconfig(e.target.value);
            setFileName(null);
          }}
          onBlur={() => void inspect(kubeconfig)}
          aria-label="Paste kubeconfig"
        />
        {inspectProblem && (
          <div className={styles.problem} role="alert">
            <div className={styles.problemTitle}>{inspectProblem.title}</div>
            <div className={styles.problemDetail}>{inspectProblem.detail}</div>
          </div>
        )}
      </section>

      {contexts && (
        <section className={styles.step} aria-label="Step 2: choose contexts">
          <h2 className={styles.stepTitle}>
            <span className={styles.stepNumber}>2</span> Choose what to import
          </h2>
          <p className={styles.hint}>
            {contexts.length === 1
              ? "This kubeconfig has one context."
              : `This kubeconfig has ${contexts.length} contexts. Each selected context becomes its own cluster.`}
          </p>
          {contexts.some((c) => c.unsupported) && (
            <p className={styles.hint}>
              Contexts whose credentials Omastx can't resolve are disabled. Re-export them
              with the client certificate or bearer token embedded in the file to import
              them.
            </p>
          )}
          <div className={styles.contextList}>
            {contexts.map((c) => (
              <label
                key={c.name}
                className={[
                  styles.contextItem,
                  selected.has(c.name) ? styles.contextChecked : "",
                  c.unsupported ? styles.contextUnsupported : "",
                ].join(" ")}
              >
                <input
                  type="checkbox"
                  checked={selected.has(c.name)}
                  disabled={!!c.unsupported}
                  onChange={() => toggleContext(c.name)}
                />
                <span className={styles.contextName}>
                  {c.name}
                  {c.current ? " (current)" : ""}
                </span>
                <span className={styles.contextServer}>{c.server}</span>
                {c.unsupported && (
                  <span className={styles.contextNote}>
                    Can't be imported: it {c.unsupported}.
                  </span>
                )}
              </label>
            ))}
          </div>
          <div className={styles.actions}>
            <Button onClick={() => void checkSelected()} disabled={busy || selected.size === 0}>
              {busy && !candidates?.some((c) => !c.checking)
                ? "Checking access…"
                : "Test connection & permissions"}
            </Button>
          </div>
        </section>
      )}

      {candidates && (
        <section className={styles.step} aria-label="Step 3: review and connect">
          <h2 className={styles.stepTitle}>
            <span className={styles.stepNumber}>3</span> Review access, then connect
          </h2>
          {candidates.map((candidate, i) => (
            <div key={candidate.context.name} className={styles.candidate}>
              <div className={styles.candidateHeader}>
                <span className={styles.candidateName}>{candidate.context.name}</span>
                {candidate.connected ? (
                  <Tag tone="current">connected</Tag>
                ) : candidate.checking ? (
                  <Tag>checking…</Tag>
                ) : candidate.check?.reachable ? (
                  candidate.check.rbac.images_ok ? (
                    candidate.check.rbac.helm_ok ? (
                      <Tag tone="current">ready</Tag>
                    ) : (
                      <Tag tone="caution">images only</Tag>
                    )
                  ) : (
                    <Tag tone="alarm">not enough access</Tag>
                  )
                ) : (
                  <Tag tone="alarm">unreachable</Tag>
                )}
              </div>

              {candidate.check && !candidate.check.reachable && (
                <div className={styles.problem} role="alert">
                  <div className={styles.problemTitle}>
                    Couldn&apos;t reach {candidate.context.name}
                  </div>
                  <div className={styles.problemDetail}>
                    {candidate.check.error}. Check that the API server is reachable from this
                    host, then re-run the check.
                  </div>
                </div>
              )}

              {candidate.check?.reachable && (
                <>
                  <PermissionMatrix rbac={candidate.check.rbac} />
                  {!candidate.check.rbac.helm_ok && candidate.check.rbac.images_ok && (
                    <div className={styles.notice}>
                      Secrets access is missing, so Helm releases can&apos;t be read. This cluster
                      will run in images-only mode; grant get/list on secrets to enable Helm data.
                    </div>
                  )}
                  {!candidate.check.rbac.images_ok && (
                    <div className={styles.problem} role="alert">
                      <div className={styles.problemTitle}>Not enough permissions to scan</div>
                      <div className={styles.problemDetail}>
                        This kubeconfig can&apos;t get/list workloads. Grant read access to pods,
                        namespaces, deployments, statefulsets, daemonsets and cronjobs, then re-run
                        the check. Omastx never needs write access.
                      </div>
                    </div>
                  )}
                  {candidate.check.rbac.images_ok && !candidate.connected && (
                    <div className={styles.fields}>
                      <Field
                        label="Cluster name"
                        value={candidate.name}
                        onChange={(e) => updateCandidate(i, { name: e.target.value })}
                      />
                      <Field
                        label="Scan schedule (cron)"
                        value={candidate.schedule}
                        hint="Default: every 6 hours"
                        onChange={(e) => updateCandidate(i, { schedule: e.target.value })}
                      />
                    </div>
                  )}
                </>
              )}

              {candidate.problem && (
                <div className={styles.problem} role="alert">
                  <div className={styles.problemTitle}>{candidate.problem.title}</div>
                  <div className={styles.problemDetail}>{candidate.problem.detail}</div>
                </div>
              )}
            </div>
          ))}

          <div className={styles.actions}>
            <Button
              onClick={() => void connectAll()}
              disabled={busy || connectable.length === 0}
            >
              {connectable.length > 1 ? `Connect ${connectable.length} clusters` : "Connect"}
            </Button>
            <Button variant="quiet" onClick={() => void navigate({ to: "/" })}>
              Cancel
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}
