import { useEffect, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { isDevAutoLogin, tryAutoDevLogin } from "./devLogin.ts";
import { Button } from "../../ui/index.ts";
import { AmmoniteMark } from "../../ui/AmmoniteMark.tsx";
import styles from "./SignInPage.module.css";

const signInErrors: Record<string, { title: string; detail: string }> = {
  not_authorized: {
    title: "You're not allowed to sign in",
    detail:
      "Use a GitHub account that is an active member of the configured organization, or matches the configured solo username.",
  },
  oauth_denied: {
    title: "GitHub sign-in was cancelled",
    detail: "Authorize Omastx on GitHub to continue, or try again.",
  },
  oauth_failed: {
    title: "GitHub sign-in failed",
    detail: "Something went wrong talking to GitHub. Try again in a moment.",
  },
};

export function SignInPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const search = useSearch({ from: "/signin" }) as { error?: string | undefined };
  const [busy, setBusy] = useState(isDevAutoLogin());

  useEffect(() => {
    if (!isDevAutoLogin()) {
      return;
    }
    let cancelled = false;
    void (async () => {
      if (await tryAutoDevLogin(queryClient)) {
        if (!cancelled) {
          await navigate({ to: "/" });
        }
        return;
      }
      if (!cancelled) {
        setBusy(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [navigate, queryClient]);

  const problem = search.error ? signInErrors[search.error] : null;

  function onGitHubSignIn() {
    window.location.href = "/api/auth/github/login";
  }

  return (
    <div className={styles.page}>
      <main className={styles.card}>
        <div className={styles.brand}>
          <AmmoniteMark size={40} />
          <div>
            <h1 className={styles.name}>Omastx</h1>
            <p className={styles.tagline}>How far has your fleet drifted?</p>
          </div>
        </div>
        <div className={styles.form}>
          {problem && (
            <div className={styles.error} role="alert">
              <div className={styles.errorTitle}>{problem.title}</div>
              <div className={styles.errorDetail}>{problem.detail}</div>
            </div>
          )}
          <Button type="button" disabled={busy} onClick={onGitHubSignIn}>
            {busy ? "Opening…" : "Sign in with GitHub"}
          </Button>
        </div>
      </main>
    </div>
  );
}
