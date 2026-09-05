import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { api, ModuleManifest, User } from "../api";
import { PublicLogin, Shell } from "../components/Shell";
import { LoadingState } from "../components/States";
import { resourceRoute } from "../routing";

const Activity = lazy(() =>
  import("../modules/Activity").then((module) => ({
    default: module.Activity,
  })),
);
const Accounts = lazy(() =>
  import("../modules/Accounts").then((module) => ({
    default: module.Accounts,
  })),
);
const Contacts = lazy(() =>
  import("../modules/Contacts").then((module) => ({
    default: module.Contacts,
  })),
);
const Documents = lazy(() =>
  import("../modules/Documents").then((module) => ({
    default: module.Documents,
  })),
);
const Signing = lazy(() => import("../modules/Signing").then((module) => ({ default: module.Signing })));
const PublicSigning = lazy(() => import("../modules/PublicSigning").then((module) => ({ default: module.PublicSigning })));
const Opportunities = lazy(() =>
  import("../modules/Opportunities").then((module) => ({
    default: module.Opportunities,
  })),
);
const Overview = lazy(() =>
  import("../modules/Overview").then((module) => ({
    default: module.Overview,
  })),
);
const SearchResults = lazy(() =>
  import("../modules/SearchResults").then((module) => ({
    default: module.SearchResults,
  })),
);
const Settings = lazy(() =>
  import("../modules/Settings").then((module) => ({
    default: module.Settings,
  })),
);
const Communications = lazy(() =>
  import("../modules/Communications").then((module) => ({
    default: module.Communications,
  })),
);
const Operations = lazy(() =>
  import("../modules/Operations").then((module) => ({
    default: module.Operations,
  })),
);
const LeadIntake = lazy(() =>
  import("../modules/LeadIntake").then((module) => ({
    default: module.LeadIntake,
  })),
);

type LocationState = { path: string; search: string };

export function App() {
  const [user, setUser] = useState<User | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);
  const [modules, setModules] = useState<ModuleManifest[]>([]);
  const [location, setLocation] = useState<LocationState>(() =>
    currentLocation(),
  );

  useEffect(() => {
    if (window.location.pathname === "/sign") {
      setCheckingSession(false);
      return;
    }
    fetch("/api/v1/me")
      .then((response) => (response.ok ? response.json() : null))
      .then(async (current: User | null) => {
        setUser(current);
        if (current) {
          const catalog = await api<{ modules: ModuleManifest[] }>(
            "/api/v1/modules",
          );
          setModules(catalog.modules ?? []);
        }
      })
      .catch(() => setUser(null))
      .finally(() => setCheckingSession(false));
  }, []);

  useEffect(() => {
    const sync = () => setLocation(currentLocation());
    window.addEventListener("popstate", sync);
    return () => window.removeEventListener("popstate", sync);
  }, []);

  const navigate = useCallback((target: string) => {
    window.history.pushState({}, "", target);
    setLocation(currentLocation());
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  async function logout() {
    await api("/auth/logout", { method: "POST" });
    setUser(null);
    window.history.replaceState({}, "", "/");
    setLocation(currentLocation());
  }

  if (location.path === "/sign") return <Suspense fallback={<LoadingState label="Opening your document" />}><PublicSigning /></Suspense>;
  if (checkingSession)
    return (
      <div className="session-loading">
        <LoadingState label="Opening Kosmos" />
      </div>
    );
  if (!user) return <PublicLogin />;

  return (
    <Shell
      user={user}
      modules={modules}
      path={basePath(location.path)}
      navigate={navigate}
      logout={logout}
    >
      <Suspense fallback={<LoadingState />}>
        <Route location={location} user={user} navigate={navigate} />
      </Suspense>
    </Shell>
  );
}

function Route({
  location,
  user,
  navigate,
}: {
  location: LocationState;
  user: User;
  navigate: (path: string) => void;
}) {
  const path = basePath(location.path);
  if (location.path === "/documents/signing" || location.path.startsWith("/documents/signing/"))
    return <Signing id={location.path.split("/")[3] ?? ""} navigate={navigate} />;
  if (path === "/")
    return (
      <Overview
        user={user}
        navigate={navigate}
        route={resourceRoute(location.path, "/landing-zone")}
      />
    );
  if (path === "/contacts")
    return (
      <Contacts
        route={resourceRoute(location.path, "/contacts")}
        navigate={navigate}
      />
    );
  if (path === "/lead") return <LeadIntake navigate={navigate} />;
  if (path === "/accounts")
    return (
      <Accounts
        route={resourceRoute(location.path, "/accounts")}
        navigate={navigate}
      />
    );
  if (path === "/opportunities") {
    const requested = new URLSearchParams(location.search).get("view");
    const initialView =
      requested === "won" || requested === "lost" ? requested : "pipeline";
    return (
      <Opportunities
        initialView={initialView}
        navigate={navigate}
        route={resourceRoute(location.path, "/opportunities")}
      />
    );
  }
  if (path === "/documents")
    return (
      <Documents
        route={resourceRoute(location.path, "/documents")}
        accountID={new URLSearchParams(location.search).get("account") ?? ""}
        navigate={navigate}
      />
    );
  if (path === "/costs" || path === "/operations")
    return (
      <Operations
        route={resourceRoute(location.path, "/operations/costs")}
        navigate={navigate}
      />
    );
  if (path === "/activity")
    return (
      <Activity
        futureOnly={location.path === "/activity/future"}
        navigate={navigate}
      />
    );
  if (path === "/communications")
    return (
      <Communications
        route={resourceRoute(location.path, "/communications/templates")}
        navigate={navigate}
      />
    );
  if (path === "/settings") return <Settings user={user} />;
  if (path === "/search")
    return (
      <SearchResults
        query={new URLSearchParams(location.search).get("q") ?? ""}
        navigate={navigate}
      />
    );
  return (
    <Overview
      user={user}
      navigate={navigate}
      route={resourceRoute("/", "/landing-zone")}
    />
  );
}

function currentLocation(): LocationState {
  return { path: window.location.pathname, search: window.location.search };
}

function basePath(path: string) {
  if (path.startsWith("/landing-zone/")) return "/";
  if (path.startsWith("/contacts/")) return "/contacts";
  if (path.startsWith("/accounts/")) return "/accounts";
  if (path.startsWith("/opportunities/")) return "/opportunities";
  if (path.startsWith("/documents/")) return "/documents";
  if (path.startsWith("/communications/")) return "/communications";
  if (path.startsWith("/operations/")) return "/operations";
  if (path.startsWith("/activity/")) return "/activity";
  return path;
}
