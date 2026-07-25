import type { QueryClient } from "@tanstack/react-query";
import { api, setCsrfToken } from "../../lib/api.ts";
import { meQuery } from "./auth.ts";

export function isDevAutoLogin(): boolean {
  // Vite dev server (make dev-local): auto sign-in unless explicitly disabled.
  if (!import.meta.env.DEV) {
    return false;
  }
  return import.meta.env.VITE_OMASTX_DEV !== "false";
}

/** Passwordless dev sign-in when the backend runs with OMASTX_DEV=true. */
export async function tryAutoDevLogin(queryClient: QueryClient): Promise<boolean> {
  if (!isDevAutoLogin()) {
    return false;
  }
  try {
    const auth = await api.devLogin();
    setCsrfToken(auth.csrf_token);
    queryClient.setQueryData(meQuery.queryKey, auth);
    return true;
  } catch {
    return false;
  }
}
