import { FormEvent, ReactNode, useState } from "react";
import {
  Bell,
  Building2,
  CheckCircle2,
  CircleDollarSign,
  FileText,
  Inbox,
  LayoutGrid,
  LogOut,
  Search,
  Settings2,
  SlidersHorizontal,
  Users,
  Waves,
} from "lucide-react";
import type { ModuleManifest, User } from "../api";

type Navigate = (path: string) => void;

const icons = {
  overview: LayoutGrid,
  contacts: Users,
  accounts: Building2,
  pipeline: CircleDollarSign,
  documents: FileText,
  costs: CheckCircle2,
  inbox: Inbox,
  operations: SlidersHorizontal,
  settings: Settings2,
};
const fallbackNavigation = [
  { path: "/", label: "Overview", icon: "overview" },
  { path: "/contacts", label: "Contacts", icon: "contacts" },
  { path: "/accounts", label: "Accounts", icon: "accounts" },
  { path: "/opportunities", label: "Opportunities", icon: "pipeline" },
  { path: "/documents", label: "Documents", icon: "documents" },
  { path: "/communications", label: "Inbox", icon: "inbox" },
  { path: "/operations", label: "Operations", icon: "operations" },
  { path: "/settings", label: "Settings", icon: "settings" },
];

export function Shell({
  user,
  modules,
  path,
  navigate,
  logout,
  children,
}: {
  user: User;
  modules: ModuleManifest[];
  path: string;
  navigate: Navigate;
  logout: () => void;
  children: ReactNode;
}) {
  const [query, setQuery] = useState("");
  const navigation = modules.length
    ? modules.flatMap((module) => module.navigation)
    : fallbackNavigation;
  function search(event: FormEvent) {
    event.preventDefault();
    const value = query.trim();
    if (value) navigate(`/search?q=${encodeURIComponent(value)}`);
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a
          className="brand"
          aria-label="Kosmos"
          href="/"
          onClick={(event) => {
            event.preventDefault();
            navigate("/");
          }}
        >
          <span className="brand-mark">
            <Waves size={20} />
          </span>
          <span className="brand-name">Kosmos</span>
        </a>
        <p className="eyebrow sidebar-label">Your workspace</p>
        <nav className="nav-list" aria-label="Workspace">
          {navigation.map(({ path: target, label, icon }) => {
            const Icon = icons[icon as keyof typeof icons] ?? LayoutGrid;
            return (
              <a
                key={target}
                aria-label={label}
                className={`nav-item ${path === target ? "active" : ""}`}
                href={target}
                onClick={(event) => {
                  event.preventDefault();
                  navigate(target);
                }}
              >
                <Icon size={18} />
                <span className="nav-label">{label}</span>
              </a>
            );
          })}
        </nav>
        <div className="sidebar-spacer" />
        <div className="user-chip">
          <UserAvatar user={user} />
          <span>
            <strong>{user.name}</strong>
            <small>{user.email}</small>
          </span>
        </div>
      </aside>
      <main className="main-content">
        <header className="topbar">
          <form className="search" role="search" onSubmit={search}>
            <Search size={18} />
            <input
              aria-label="Search Kosmos"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search people, documents, and costs"
            />
          </form>
          <div className="top-actions">
            <button
              className="icon-button"
              aria-label="Notifications and follow-ups"
              onClick={() => navigate("/activity")}
            >
              <Bell size={19} />
            </button>
            <button className="account-button" aria-label="Sign out" title="Sign out" onClick={logout}>
              <LogOut size={19} aria-hidden="true" />
              <span>Sign out</span>
            </button>
          </div>
        </header>
        <div className="content-wrap">{children}</div>
      </main>
    </div>
  );
}

export function PublicLogin() {
  return (
    <main className="public-login">
      <div className="public-atmosphere" aria-hidden="true">
        <span />
        <span />
        <span />
      </div>
      <section className="public-copy">
        <div className="public-brand">
          <span className="brand-mark">
            <Waves size={22} />
          </span>
          <span>Kosmos</span>
        </div>
        <p className="eyebrow">Nerds Who Fish workspace</p>
        <h1>
          Your business,
          <br />
          <em>without the busywork.</em>
        </h1>
        <p className="public-lede">
          One calm place for people, opportunities, notes, documents,
          follow-ups, and the money keeping the lights on.
        </p>
        <a className="public-signin" href="/auth/login">
          <span>Continue with Google</span>
          <span aria-hidden="true">→</span>
        </a>
        <p className="public-security">
          <span className="security-dot" />
          Access is limited to approved company Google accounts.
        </p>
      </section>
      <aside
        className="public-orbit"
        aria-label="Kosmos organizes your business"
      >
        <div className="orbit-core">
          <Waves size={34} />
          <strong>
            Your work,
            <br />
            in orbit.
          </strong>
        </div>
        <span className="orbit-chip orbit-people">People</span>
        <span className="orbit-chip orbit-work">Next steps</span>
        <span className="orbit-chip orbit-knowledge">Knowledge</span>
        <span className="orbit-chip orbit-money">Costs</span>
      </aside>
    </main>
  );
}

function initials(name: string) {
  return name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

function UserAvatar({ user }: { user: User }) {
  const [failed, setFailed] = useState(false);
  return (
    <span className="avatar">
      {user.picture && !failed ? (
        <img
          src={user.picture}
          alt=""
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
        />
      ) : (
        initials(user.name)
      )}
    </span>
  );
}
