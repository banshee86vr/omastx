import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken } from "../lib/api.ts";
import { meQuery } from "../features/auth/auth.ts";
import { Button, useToast } from "../ui/index.ts";
import { AmmoniteMark } from "../ui/AmmoniteMark.tsx";
import styles from "./AppFrame.module.css";

export function AppFrame() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toastError } = useToast();
  const { data: auth } = useQuery(meQuery);

  async function onSignOut() {
    try {
      await api.logout();
      setCsrfToken(null);
      queryClient.removeQueries({ queryKey: meQuery.queryKey });
      await navigate({ to: "/signin" });
    } catch {
      toastError("Couldn't sign you out", "The server didn't respond. Try again.");
    }
  }

  return (
    <div className={styles.frame}>
      <nav className={styles.rail} aria-label="Main">
        <Link to="/" className={styles.brand}>
          <AmmoniteMark size={28} />
          Omastx
        </Link>

        <div className={styles.manifest}>
          <div className={styles.sectionTitle}>Manifest</div>
          <p className={styles.manifestEmpty}>No clusters yet.</p>
        </div>

        <div className={styles.nav}>
          <Link to="/" className={styles.navLink}>
            Fleet
          </Link>
        </div>

        {auth && (
          <div className={styles.user}>
            <span className={styles.email}>{auth.user.email}</span>
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
