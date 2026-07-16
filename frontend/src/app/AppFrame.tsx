import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken } from "../lib/api.ts";
import { meQuery } from "../features/auth/auth.ts";
import { clustersQuery } from "../features/clusters/clusters.ts";
import { Button, Flag, useToast, type FlagTone } from "../ui/index.ts";
import { AmmoniteMark } from "../ui/AmmoniteMark.tsx";
import styles from "./AppFrame.module.css";

function flagTone(status: string): FlagTone {
  switch (status) {
    case "connected":
      return "current";
    case "degraded":
      return "caution";
    case "error":
      return "alarm";
    default:
      return "unknown";
  }
}

export function AppFrame() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toastError } = useToast();
  const { data: auth } = useQuery(meQuery);
  const { data: clusters } = useQuery(clustersQuery);

  async function onSignOut() {
    try {
      await api.logout();
      setCsrfToken(null);
      queryClient.removeQueries({ queryKey: meQuery.queryKey });
      await navigate({ to: "/signin", search: { error: undefined } });
    } catch {
      toastError("Couldn't sign you out", "The server didn't respond. Try again.");
    }
  }

  const displayName = auth?.user.name?.trim();
  const login = auth?.user.login;

  return (
    <div className={styles.frame}>
      <nav className={styles.rail} aria-label="Main">
        <Link to="/" className={styles.brand}>
          <AmmoniteMark size={28} />
          Omastx
        </Link>

        <div className={styles.manifest}>
          <div className={styles.sectionTitle}>Manifest</div>
          {clusters && clusters.length > 0 ? (
            <div className={styles.manifestList}>
              {clusters.map((c) => (
                <Link
                  key={c.id}
                  to="/clusters/$clusterId"
                  params={{ clusterId: c.id }}
                  className={styles.manifestItem}
                >
                  <Flag tone={flagTone(c.status)} label={c.name} />
                </Link>
              ))}
            </div>
          ) : (
            <p className={styles.manifestEmpty}>No clusters yet.</p>
          )}
          <Link to="/clusters/new" className={styles.connectLink}>
            + Connect cluster
          </Link>
        </div>

        <div className={styles.nav}>
          <Link to="/" className={styles.navLink}>
            Fleet
          </Link>
          <Link to="/artifacts" className={styles.navLink}>
            Artifacts
          </Link>
          <Link to="/settings" className={styles.navLink}>
            Settings
          </Link>
        </div>

        {auth && (
          <div className={styles.user}>
            <div className={styles.userIdentity}>
              {auth.user.avatar_url ? (
                <img
                  className={styles.avatar}
                  src={auth.user.avatar_url}
                  alt=""
                  width={32}
                  height={32}
                />
              ) : (
                <div className={styles.avatarPlaceholder} aria-hidden="true" />
              )}
              <div className={styles.userMeta}>
                {displayName ? (
                  <span className={styles.displayName}>{displayName}</span>
                ) : null}
                <span className={styles.login}>{login}</span>
              </div>
            </div>
            <Button variant="quiet" onClick={onSignOut}>
              Sign out
            </Button>
          </div>
        )}
      </nav>
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  );
}
