import { useCallback, useEffect, useState } from 'react'
import { BookOpen, CalendarCheck2, Check, Clock3, Mail, Phone } from 'lucide-react'
import { Activity as ActivityRecord, api, Reminder, shortDate } from '../api'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function Activity() {
  const [activities, setActivities] = useState<ActivityRecord[]>([])
  const [reminders, setReminders] = useState<Reminder[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const load = useCallback(() => {
    setLoading(true)
    Promise.all([api<{ activities: ActivityRecord[] }>('/api/v1/activities'), api<{ reminders: Reminder[] }>('/api/v1/reminders')])
      .then(([activityResponse, reminderResponse]) => { setActivities(activityResponse.activities); setReminders(reminderResponse.reminders) })
      .catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  async function complete(item: Reminder) {
    try {
      const updated = await api<Reminder>(`/api/v1/reminders/${item.id}`, { method: 'PATCH', body: JSON.stringify({ completed: true }) })
      setReminders((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Could not complete reminder')
    }
  }

  if (loading) return <LoadingState label="Loading activity" />
  if (error) return <ErrorState message={error} retry={load} />
  const pending = reminders.filter((item) => !item.completed)

  return <Page eyebrow="Attention" title="Activity and follow-ups" detail="A single queue for what happened and what needs to happen next.">
    <div className="activity-page-grid"><section className="panel"><div className="panel-heading"><div><p className="eyebrow">Up next</p><h2>Follow-ups</h2></div><span className="count-chip">{pending.length}</span></div>{pending.length ? <div className="reminder-list">{pending.map((item) => <article key={item.id}><span className="activity-icon"><Clock3 size={17} /></span><span><strong>{item.title}</strong><small>Due {shortDate(item.dueAt)}</small></span><button aria-label={`Complete ${item.title}`} onClick={() => complete(item)}><Check size={17} /></button></article>)}</div> : <EmptyState title="You are caught up" detail="New follow-up reminders will collect here." />}</section><section className="panel"><div className="panel-heading"><div><p className="eyebrow">History</p><h2>Everything that happened</h2></div></div>{activities.length ? <div className="timeline">{activities.map((item) => <div className="timeline-item" key={item.id}><span className="timeline-icon">{activityIcon(item.kind)}</span><div><strong>{item.kind}</strong><p>{item.body}</p><time>{shortDate(item.occurredAt)}</time></div></div>)}</div> : <EmptyState title="No activity yet" detail="Notes, calls, emails, and meetings added to contacts will appear here." />}</section></div>
  </Page>
}

function activityIcon(kind: ActivityRecord['kind']) {
  if (kind === 'call') return <Phone size={16} />
  if (kind === 'email') return <Mail size={16} />
  if (kind === 'meeting') return <CalendarCheck2 size={16} />
  return <BookOpen size={16} />
}
