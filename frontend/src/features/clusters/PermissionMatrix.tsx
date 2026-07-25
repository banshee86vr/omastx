import type { RBACReport } from "../../lib/api.ts";
import styles from "./PermissionMatrix.module.css";

/** Verb-by-resource matrix from the RBAC self-check (SPEC §5.3). */
export function PermissionMatrix({ rbac }: { rbac: RBACReport }) {
  const permissions = rbac.permissions ?? [];
  const resources = [...new Set(permissions.map((p) => `${p.group}/${p.resource}`))];

  const allowed = (key: string, verb: string) =>
    permissions.find((p) => `${p.group}/${p.resource}` === key && p.verb === verb)?.allowed ??
    false;

  return (
    <div className={styles.matrix} role="table" aria-label="Granted permissions">
      <div className={[styles.row, styles.head].join(" ")} role="row">
        <span className={styles.cell} role="columnheader">
          resource
        </span>
        <span className={styles.cell} role="columnheader">
          get
        </span>
        <span className={styles.cell} role="columnheader">
          list
        </span>
      </div>
      {resources.map((key) => (
        <div key={key} className={styles.row} role="row">
          <span className={[styles.cell, styles.resource].join(" ")} role="cell">
            {key.startsWith("/") ? key.slice(1) : key}
          </span>
          {["get", "list"].map((verb) => (
            <span
              key={verb}
              className={[styles.cell, allowed(key, verb) ? styles.allowed : styles.denied].join(
                " ",
              )}
              role="cell"
            >
              {allowed(key, verb) ? "✓ allowed" : "✗ denied"}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}
