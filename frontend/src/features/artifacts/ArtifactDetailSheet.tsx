import { useQuery } from "@tanstack/react-query";
import { Sheet, Tag } from "../../ui/index.ts";
import { ApiError } from "../../lib/api.ts";
import { artifactQuery, driftTone, isUnverifiedMatch, kindLabel } from "./artifacts.ts";
import styles from "./ArtifactDetailSheet.module.css";

interface Props {
  artifactId: string | null;
  onClose: () => void;
}

function metaString(meta: Record<string, unknown> | null | undefined, key: string): string | null {
  const v = meta?.[key];
  return typeof v === "string" && v.trim() !== "" ? v : null;
}

function firstRepoLikeSource(meta: Record<string, unknown> | null | undefined): string | null {
  const sources = meta?.sources;
  if (!Array.isArray(sources)) {
    return null;
  }
  for (const item of sources) {
    if (typeof item !== "string") {
      continue;
    }
    const s = item.trim();
    if (s.startsWith("http://") || s.startsWith("https://") || s.startsWith("oci://")) {
      return s;
    }
  }
  return null;
}

function registryURLFor(kind: string, meta: Record<string, unknown> | null | undefined): string | null {
  if (kind === "image") {
    return metaString(meta, "registry");
  }
  return (
    metaString(meta, "chart_repo") ??
    metaString(meta, "auth_target") ??
    firstRepoLikeSource(meta)
  );
}

function artifactHubURLFor(kind: string, meta: Record<string, unknown> | null | undefined): string | null {
  return kind === "helm" ? metaString(meta, "artifacthub_url") : null;
}

export function ArtifactDetailSheet({ artifactId, onClose }: Props) {
  const { data, isLoading, error } = useQuery({
    ...artifactQuery(artifactId ?? ""),
    enabled: artifactId !== null,
  });

  const sourceMeta = data?.source_meta ?? undefined;
  const registryURL = data ? registryURLFor(data.kind, sourceMeta) : null;
  const artifactHubURL = data ? artifactHubURLFor(data.kind, sourceMeta) : null;
  const home = metaString(sourceMeta, "home");
  const release = metaString(sourceMeta, "release");
  const authRequired = metaString(sourceMeta, "resolve_status") === "auth_required";
  const repoUnknown = metaString(sourceMeta, "resolve_status") === "repo_unknown";
  const authDetail = metaString(sourceMeta, "resolve_detail");
  const authTarget = metaString(sourceMeta, "auth_target");

  return (
    <Sheet title={data?.identity ?? "Artifact"} open={artifactId !== null} onClose={onClose}>
      {isLoading && <p className={styles.muted}>Loading artifact…</p>}
      {error && (
        <p className={styles.muted}>
          {error instanceof ApiError ? error.problem.detail : "Couldn't load this artifact."}
        </p>
      )}
      {data && (
        <div className={styles.body}>
          <div className={styles.versions}>
            <span className={styles.installed}>{data.installed}</span>
            <span className={styles.arrow} aria-hidden="true">
              →
            </span>
            <span className={styles.latest}>{data.latest ?? "unknown"}</span>
            <Tag tone={driftTone(data.drift_class)}>{data.drift_class}</Tag>
            {data.kind === "helm" && isUnverifiedMatch(data.confidence) && (
              <Tag tone="fathom">unverified match</Tag>
            )}
          </div>

          {(authRequired || repoUnknown) && (
            <div className={styles.authNotice} role="status">
              <p>
                {authDetail ??
                  (repoUnknown
                    ? "This chart's repository URL isn't in the release metadata. Add your private Helm chart repo on the cluster page, then re-scan."
                    : "This artifact needs registry credentials to resolve latest.")}
              </p>
              {authTarget && <p className={styles.muted}>Target: {authTarget}</p>}
              {!repoUnknown && (
                <p className={styles.muted}>
                  For images, add an imagePullSecret on the workload or configure a pull secret on
                  the cluster page. For Helm, add chart repo credentials there, then re-scan.
                </p>
              )}
            </div>
          )}

          <dl className={styles.meta}>
            <dt>Kind</dt>
            <dd>{kindLabel(data.kind)}</dd>
            <dt>Cluster</dt>
            <dd>{data.cluster_name}</dd>
            <dt>Namespace</dt>
            <dd>{data.namespace}</dd>
            <dt>{data.kind === "helm" ? "Release" : "Workload"}</dt>
            <dd>
              {data.kind === "helm" && release
                ? release
                : `${data.owner_kind} ${data.owner_name}`}
            </dd>
            {registryURL && (
              <>
                <dt>Registry URL</dt>
                <dd>
                  {registryURL.startsWith("http") ? (
                    <a className={styles.link} href={registryURL} target="_blank" rel="noreferrer">
                      {registryURL}
                    </a>
                  ) : (
                    registryURL
                  )}
                </dd>
              </>
            )}
            {data.kind === "helm" && home && (
              <>
                <dt>Home</dt>
                <dd>
                  <a className={styles.link} href={home} target="_blank" rel="noreferrer">
                    {home}
                  </a>
                </dd>
              </>
            )}
            {artifactHubURL && (
              <>
                <dt>Artifact Hub</dt>
                <dd>
                  <a className={styles.link} href={artifactHubURL} target="_blank" rel="noreferrer">
                    View on Artifact Hub
                  </a>
                </dd>
              </>
            )}
            {data.kind === "helm" && data.confidence != null && data.confidence > 0 && (
              <>
                <dt>Match confidence</dt>
                <dd>{Math.round(data.confidence * 100)}%</dd>
              </>
            )}
            {data.releases_behind !== null && (
              <>
                <dt>Releases behind</dt>
                <dd>{data.releases_behind}</dd>
              </>
            )}
            <dt>First seen</dt>
            <dd>{new Date(data.first_seen).toLocaleString()}</dd>
            <dt>Last seen</dt>
            <dd>{new Date(data.last_seen).toLocaleString()}</dd>
          </dl>

          <section className={styles.section}>
            <h3 className={styles.sectionTitle}>Candidate versions</h3>
            {data.candidates.length > 0 ? (
              <ul className={styles.candidates}>
                {data.candidates.map((tag) => (
                  <li
                    key={tag}
                    className={tag === data.installed ? styles.candidateCurrent : undefined}
                  >
                    {tag}
                    {tag === data.installed && <span className={styles.badge}> installed</span>}
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.muted}>
                No comparable versions found. The installed version isn&apos;t semver-comparable,
                so drift can&apos;t be measured.
              </p>
            )}
          </section>
        </div>
      )}
    </Sheet>
  );
}
