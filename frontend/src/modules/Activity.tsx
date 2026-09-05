import { useCallback, useEffect, useMemo, useState } from "react";
import {
  BookOpen,
  CalendarCheck2,
  Check,
  Clock3,
  Mail,
  Phone,
} from "lucide-react";
import { Activity as ActivityRecord, api, Reminder, shortDate } from "../api";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { WorkflowPage } from "../components/WorkflowPage";
import { useAsyncLoad } from "../useAsyncLoad";

const reminderWindowMilliseconds = 7 * 24 * 60 * 60 * 1000;

export function Activity({
  futureOnly,
  navigate,
}: {
  futureOnly: boolean;
  navigate: (path: string) => void;
}) {
  const [activities, setActivities] = useState<ActivityRecord[]>([]);
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const { loading, error, setError, run } = useAsyncLoad();
  const load = useCallback(() => {
    void run(
      () => Promise.all([
        api<{ activities: ActivityRecord[] }>("/api/v1/activities"),
        api<{ reminders: Reminder[] }>("/api/v1/reminders"),
      ]),
      ([activityResponse, reminderResponse]) => {
        setActivities(activityResponse.activities);
        setReminders(reminderResponse.reminders);
      },
    );
  }, [run]);
  useEffect(load, [load]);

  async function complete(item: Reminder) {
    try {
      const updated = await api<Reminder>(`/api/v1/reminders/${item.id}`, {
        method: "PATCH",
        body: JSON.stringify({ completed: true }),
      });
      setReminders((current) =>
        current.map((candidate) =>
          candidate.id === updated.id ? updated : candidate,
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not complete reminder",
      );
    }
  }

  const pending = useMemo(
    () =>
      reminders
        .filter((item) => !item.completed)
        .sort(
          (left, right) =>
            new Date(left.dueAt).getTime() - new Date(right.dueAt).getTime(),
        ),
    [reminders],
  );
  const cutoff = Date.now() + reminderWindowMilliseconds;
  const dueSoon = pending.filter(
    (item) => new Date(item.dueAt).getTime() <= cutoff,
  );
  const future = pending.filter(
    (item) => new Date(item.dueAt).getTime() > cutoff,
  );

  if (loading) return <LoadingState label="Loading activity" />;
  if (error) return <ErrorState message={error} retry={load} />;

  if (futureOnly)
    return (
      <WorkflowPage
        eyebrow="Later"
        title="Future reminders"
        detail="Everything more than one week away, sorted by due date."
        backLabel="Back to activity"
        onBack={() => navigate("/activity")}
      >
        {future.length ? (
          <ReminderList reminders={future} complete={complete} />
        ) : (
          <EmptyState
            title="Nothing waiting"
            detail="Reminders more than one week away will collect here."
          />
        )}
      </WorkflowPage>
    );

  return (
    <Page
      eyebrow="Attention"
      title="Activity and follow-ups"
      detail="A single queue for what happened and what needs to happen next."
    >
      <div className="activity-page-grid">
        <section className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Up next</p>
              <h2>Follow-ups</h2>
            </div>
            <div className="activity-header-actions">
              <button
                className="text-button"
                onClick={() => navigate("/activity/future")}
              >
                Future reminders{" "}
                <span className="count-chip">{future.length}</span>
              </button>
              <span className="count-chip">{dueSoon.length}</span>
            </div>
          </div>
          {dueSoon.length ? (
            <ReminderList reminders={dueSoon} complete={complete} />
          ) : (
            <EmptyState
              title="You are caught up"
              detail="Reminders appear here one week before they are due."
            />
          )}
        </section>
        <section className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">History</p>
              <h2>Everything that happened</h2>
            </div>
          </div>
          {activities.length ? (
            <div className="timeline">
              {activities.map((item) => (
                <div className="timeline-item" key={item.id}>
                  <span className="timeline-icon">
                    {activityIcon(item.kind)}
                  </span>
                  <div>
                    <strong>{item.kind}</strong>
                    <p>{item.body}</p>
                    <time>{shortDate(item.occurredAt)}</time>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState
              title="No activity yet"
              detail="Notes, calls, emails, and meetings added to contacts will appear here."
            />
          )}
        </section>
      </div>
    </Page>
  );
}

function ReminderList({
  reminders,
  complete,
}: {
  reminders: Reminder[];
  complete: (item: Reminder) => void;
}) {
  return (
    <div className="reminder-list">
      {reminders.map((item) => (
        <article key={item.id}>
          <span className="activity-icon">
            <Clock3 size={17} />
          </span>
          <span>
            <strong>{item.title}</strong>
            <small>Due {shortDate(item.dueAt)}</small>
          </span>
          <button
            aria-label={`Complete ${item.title}`}
            onClick={() => complete(item)}
          >
            <Check size={17} />
          </button>
        </article>
      ))}
    </div>
  );
}

function activityIcon(kind: ActivityRecord["kind"]) {
  if (kind === "call") return <Phone size={16} />;
  if (kind === "email") return <Mail size={16} />;
  if (kind === "meeting") return <CalendarCheck2 size={16} />;
  return <BookOpen size={16} />;
}
