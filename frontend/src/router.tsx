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
import { meQuery } from "./features/auth/auth.ts";

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

const routeTree = rootRoute.addChildren([signInRoute, authedRoute.addChildren([fleetRoute])]);

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
