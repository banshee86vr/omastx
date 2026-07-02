import { useQuery } from "@tanstack/react-query";
import { Sheet, Tag } from "../../ui/index.ts";
import { ApiError } from "../../lib/api.ts";
import { artifactQuery, driftTone } from "./artifacts.ts";
import styles from "./ArtifactDetailSheet.module.css";

interface Props {
  artifactId: string | null;
  onClose: () => void;
}

export function ArtifactDetailSheet({ artifactId, onClose }: Props) {
  const { data, isLoading, error } = useQuery({
    ...artifactQuery(artifactId ?? ""),
    enabled: artifactId !== null,
  });

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
          </div>

          <dl className={styles.meta}>
            <dt>Cluster</dt>
            <dd>{data.cluster_name}</dd>
            <dt>Namespace</dt>
            <dd>{data.namespace}</dd>
            <dt>Workload</dt>
            <dd>
              {data.owner_kind} {data.owner_name}
            </dd>
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
                No comparable versions found. The installed tag isn&apos;t semver-comparable
                (a digest, date, or moving tag), so drift can&apos;t be measured.
              </p>
            )}
          </section>
        </div>
      )}
    </Sheet>
  );
}
