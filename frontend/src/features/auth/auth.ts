import { queryOptions } from "@tanstack/react-query";
import { api, setCsrfToken, type AuthResponse } from "../../lib/api.ts";

export const meQuery = queryOptions({
  queryKey: ["auth", "me"],
  queryFn: async (): Promise<AuthResponse> => {
    const auth = await api.me();
    setCsrfToken(auth.csrf_token);
    return auth;
  },
  retry: false,
  staleTime: 5 * 60 * 1000,
});
