import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken, ApiError, type Problem } from "../../lib/api.ts";
import { meQuery } from "./auth.ts";
import { isDevAutoLogin, tryAutoDevLogin } from "./devLogin.ts";
import { Button, Field } from "../../ui/index.ts";
import { AmmoniteMark } from "../../ui/AmmoniteMark.tsx";
import styles from "./SignInPage.module.css";

export function SignInPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [problem, setProblem] = useState<Problem | null>(null);
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

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setProblem(null);
    try {
      const auth = await api.login(email, password);
      setCsrfToken(auth.csrf_token);
      queryClient.setQueryData(meQuery.queryKey, auth);
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof ApiError) {
        setProblem(err.problem);
      } else {
        setProblem({
          code: "unreachable",
          title: "Couldn't reach the server",
          detail: "Check that the backend is running, then try again.",
        });
      }
    } finally {
      setBusy(false);
    }
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
        <form className={styles.form} onSubmit={onSubmit}>
          <Field
            label="Email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Field
            label="Password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {problem && (
            <div className={styles.error} role="alert">
              <div className={styles.errorTitle}>{problem.title}</div>
              <div className={styles.errorDetail}>{problem.detail}</div>
            </div>
          )}
          <Button type="submit" disabled={busy}>
            {busy ? (isDevAutoLogin() ? "Opening…" : "Signing in…") : "Sign in"}
          </Button>
        </form>
      </main>
    </div>
  );
}
