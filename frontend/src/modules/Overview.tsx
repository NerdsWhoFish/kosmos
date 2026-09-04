import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  ArrowUpRight,
  BookOpen,
  CalendarDays,
  Globe2,
  Mail,
  Plus,
  Users,
} from "lucide-react";
import { Activity, api, Landing, Member, money, Summary, User } from "../api";
import { Modal } from "../components/Modal";
import { ErrorState, LoadingState } from "../components/States";

const emptySummary: Summary = {
  contacts: 0,
  openOpportunities: 0,
  pipelineAmountCents: 0,
  wonOpportunities: 0,
  wonAmountCents: 0,
  lostOpportunities: 0,
  lostAmountCents: 0,
  followUpsDue: 0,
  currentMonthCostCents: 0,
  recentActivities: [],
};
const iconMap: Record<string, typeof Globe2> = {
  globe: Globe2,
  calendar: CalendarDays,
  users: Users,
};

export function Overview({
  user,
  navigate,
}: {
  user: User;
  navigate: (path: string) => void;
}) {
  const [summary, setSummary] = useState(emptySummary);
  const [landing, setLanding] = useState<Landing>({
    buttons: [],
    notifications: [],
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [shortcutOpen, setShortcutOpen] = useState(false);
  const [shortcutError, setShortcutError] = useState("");
  const [saving, setSaving] = useState(false);
  const [canManageLanding, setCanManageLanding] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setError("");
    Promise.all([
      api<Summary>("/api/v1/summary"),
      api<Landing>("/api/v1/landing"),
      api<{ notifications: Landing["notifications"] }>("/api/v1/notifications"),
      api<{ members: Member[] }>("/api/v1/members"),
    ])
      .then(([nextSummary, nextLanding, notificationResult, memberResult]) => {
        setSummary(nextSummary);
        setLanding({
          ...nextLanding,
          notifications: notificationResult.notifications,
        });
        setCanManageLanding(
          ["owner", "admin"].includes(
            memberResult.members.find(
              (member) =>
                member.email.toLowerCase() === user.email.toLowerCase(),
            )?.role ?? "",
          ),
        );
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, [user.email]);

  useEffect(load, [load]);

  async function createShortcut(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setShortcutError("");
    const form = new FormData(event.currentTarget);
    try {
      await api("/api/v1/landing/buttons", {
        method: "POST",
        body: JSON.stringify({
          label: form.get("label"),
          description: form.get("description"),
          href: form.get("href"),
        }),
      });
      setShortcutOpen(false);
      load();
    } catch (reason) {
      setShortcutError(
        reason instanceof Error ? reason.message : "Could not save shortcut",
      );
    } finally {
      setSaving(false);
    }
  }

  const today = new Intl.DateTimeFormat("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
  }).format(new Date());
  const hour = new Date().getHours();
  const greeting =
    hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  if (loading) return <LoadingState />;
  if (error) return <ErrorState message={error} retry={load} />;

  return (
    <>
      <section className="welcome-row">
        <div>
          <p className="eyebrow">{today}</p>
          <h1>
            {greeting}, {user.name.split(" ")[0] || "there"}.
          </h1>
          <p className="subhead">Here is what needs your attention.</p>
        </div>
        <button
          className="primary-button"
          onClick={() => navigate("/contacts?new=1")}
        >
          <Plus size={17} /> Add a contact
        </button>
      </section>
      <section className="stats-row">
        <Stat
          label="Open opportunities"
          value={String(summary.openOpportunities)}
          detail={`${money(summary.pipelineAmountCents)} in the pipeline`}
          tone="blue"
          onClick={() => navigate("/opportunities")}
        />
        <Stat
          label="Won opportunities"
          value={String(summary.wonOpportunities)}
          detail={`${money(summary.wonAmountCents)} won`}
          tone="green"
          onClick={() => navigate("/opportunities?view=won")}
        />
        <Stat
          label="Lost opportunities"
          value={String(summary.lostOpportunities)}
          detail={`${money(summary.lostAmountCents)} lost`}
          tone="red"
          onClick={() => navigate("/opportunities?view=lost")}
        />
        <Stat
          label="Follow-ups due"
          value={String(summary.followUpsDue)}
          detail="Keep the next conversation moving"
          tone="gold"
          onClick={() => navigate("/activity")}
        />
        <Stat
          label="This month's costs"
          value={money(summary.currentMonthCostCents)}
          detail="Recorded business spending"
          tone="green"
          onClick={() => navigate("/operations")}
        />
      </section>
      <section className="dashboard-grid">
        <div className="panel quick-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Team shortcuts</p>
              <h2>Landing zone</h2>
            </div>
            {canManageLanding && (
              <button
                className="text-button"
                onClick={() => setShortcutOpen(true)}
              >
                <Plus size={15} /> Customize
              </button>
            )}
          </div>
          <div className="shortcut-grid">
            {landing.buttons.map((button) => {
              const Icon = iconMap[button.icon] ?? Globe2;
              return (
                <a className="shortcut-card" href={button.href} key={button.id}>
                  <span className="shortcut-icon">
                    <Icon size={20} />
                  </span>
                  <span>
                    <strong>{button.label}</strong>
                    <small>{button.description}</small>
                  </span>
                  <ArrowUpRight className="shortcut-arrow" size={17} />
                </a>
              );
            })}
            {canManageLanding && (
              <button
                className="shortcut-card add-card"
                onClick={() => setShortcutOpen(true)}
              >
                <span className="shortcut-icon muted">
                  <Plus size={20} />
                </span>
                <span>
                  <strong>Add a shortcut</strong>
                  <small>Everyone in your organization will see it.</small>
                </span>
              </button>
            )}
          </div>
        </div>
        <div className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Stay in the loop</p>
              <h2>Recent activity</h2>
            </div>
            <button
              className="text-button"
              onClick={() => navigate("/activity")}
            >
              View all <ArrowUpRight size={15} />
            </button>
          </div>
          <div className="activity-list">
            {summary.recentActivities.length ? (
              summary.recentActivities.map((item) => (
                <ActivityRow key={item.id} item={item} />
              ))
            ) : (
              <div className="activity-item">
                <span className="activity-icon lavender">
                  <BookOpen size={16} />
                </span>
                <span>
                  <strong>A clean slate</strong>
                  <small>
                    Notes, calls, meetings, and emails will show up here.
                  </small>
                  <time>Add your first contact to begin</time>
                </span>
              </div>
            )}
          </div>
        </div>
      </section>
      {!!landing.notifications.length && (
        <section className="panel notification-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Across your business</p>
              <h2>What just happened</h2>
            </div>
            <button
              className="text-button"
              onClick={() => navigate("/communications")}
            >
              Open inbox <ArrowUpRight size={15} />
            </button>
          </div>
          <div className="activity-list">
            {landing.notifications.slice(0, 5).map((item) => (
              <button
                className="activity-item notification-item"
                key={item.id}
                onClick={() => navigate(item.href)}
              >
                <span className="activity-icon lavender">
                  <Mail size={16} />
                </span>
                <span>
                  <strong>{item.title}</strong>
                  <small>{item.summary}</small>
                  <time>
                    {new Intl.RelativeTimeFormat("en", {
                      numeric: "auto",
                    }).format(
                      Math.round(
                        (new Date(item.createdAt).getTime() - Date.now()) /
                          86400000,
                      ),
                      "day",
                    )}
                  </time>
                </span>
              </button>
            ))}
          </div>
        </section>
      )}
      {shortcutOpen && (
        <Modal
          eyebrow="Landing zone"
          title="Add a shortcut"
          onClose={() => setShortcutOpen(false)}
        >
          <form onSubmit={createShortcut}>
            <label>
              Button name
              <input name="label" maxLength={80} required autoFocus />
            </label>
            <label>
              Link
              <input
                name="href"
                inputMode="url"
                placeholder="https://example.com"
                required
              />
            </label>
            <label>
              Description
              <textarea name="description" maxLength={180} rows={3} />
            </label>
            {shortcutError && (
              <p className="form-error" role="alert">
                {shortcutError}
              </p>
            )}
            <div className="form-actions">
              <button
                type="button"
                className="secondary-button"
                onClick={() => setShortcutOpen(false)}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving ? "Saving..." : "Save shortcut"}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

function ActivityRow({ item }: { item: Activity }) {
  const Icon =
    item.kind === "email"
      ? Mail
      : item.kind === "meeting"
        ? CalendarDays
        : BookOpen;
  return (
    <div className="activity-item">
      <span className="activity-icon">
        <Icon size={16} />
      </span>
      <span>
        <strong>{item.kind === "note" ? "Note added" : item.kind}</strong>
        <small>{item.body}</small>
        <time>
          {new Intl.RelativeTimeFormat("en", { numeric: "auto" }).format(
            Math.round(
              (new Date(item.occurredAt).getTime() - Date.now()) / 86400000,
            ),
            "day",
          )}
        </time>
      </span>
    </div>
  );
}

function Stat({
  label,
  value,
  detail,
  tone,
  onClick,
}: {
  label: string;
  value: string;
  detail: string;
  tone: string;
  onClick: () => void;
}) {
  return (
    <button className={`stat-card ${tone}`} onClick={onClick}>
      <span className="stat-label">{label}</span>
      <strong className="stat-value">{value}</strong>
      <small>{detail}</small>
    </button>
  );
}
