import { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { AppFrame } from "./app/AppFrame.tsx";
import { SignInPage } from "./features/auth/SignInPage.tsx";
import { FleetPage } from "./features/fleet/FleetPage.tsx";
import { ConnectClusterPage } from "./features/clusters/ConnectClusterPage.tsx";
import { ClusterDetailPage } from "./features/clusters/ClusterDetailPage.tsx";
import { ArtifactsPage } from "./features/artifacts/ArtifactsPage.tsx";
import { driftClassSchema, type DriftClass } from "./lib/api.ts";
import { meQuery } from "./features/auth/auth.ts";
import { tryAutoDevLogin } from "./features/auth/devLogin.ts";

export interface ArtifactsSearch {
  cluster?: string | undefined;
  scan?: string | undefined;
  kind?: string | undefined;
  namespace?: string | undefined;
  class?: DriftClass | undefined;
  resolve_status?: string | undefined;
  q?: string | undefined;
}

interface RouterContext {
  queryClient: QueryClient;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
});

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/signin",
  component: SignInPage,
});

// Everything under this layout requires a valid session.
const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "authed",
  beforeLoad: async ({ context }) => {
    try {
      await context.queryClient.ensureQueryData(meQuery);
    } catch {
      if (await tryAutoDevLogin(context.queryClient)) {
        return;
      }
      throw redirect({ to: "/signin" });
    }
  },
  component: AppFrame,
});

const fleetRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/",
  component: FleetPage,
});

const connectClusterRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/clusters/new",
  component: ConnectClusterPage,
});

const clusterDetailRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/clusters/$clusterId",
  component: ClusterDetailPage,
});

const artifactsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/artifacts",
  validateSearch: (search: Record<string, unknown>): ArtifactsSearch => {
    const cls = driftClassSchema.safeParse(search.class);
    const kind = typeof search.kind === "string" && (search.kind === "image" || search.kind === "helm")
      ? search.kind
      : undefined;
    return {
      cluster: typeof search.cluster === "string" ? search.cluster : undefined,
      scan: typeof search.scan === "string" ? search.scan : undefined,
      kind,
      namespace: typeof search.namespace === "string" ? search.namespace : undefined,
      class: cls.success ? cls.data : undefined,
      resolve_status:
        search.resolve_status === "auth_required" ? "auth_required" : undefined,
      q: typeof search.q === "string" ? search.q : undefined,
    };
  },
  component: ArtifactsPage,
});

const routeTree = rootRoute.addChildren([
  signInRoute,
  authedRoute.addChildren([
    fleetRoute,
    connectClusterRoute,
    clusterDetailRoute,
    artifactsRoute,
  ]),
]);

export function makeRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: "intent",
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof makeRouter>;
  }
}
