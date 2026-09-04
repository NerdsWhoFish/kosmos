import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowLeft, Building2, CalendarPlus, Mail, MessageSquarePlus, Phone, Plus, UserRound } from 'lucide-react'
import { Activity, api, Contact, Opportunity, Reminder, shortDate } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function Contacts({ openNew, clearNew }: { openNew: boolean; clearNew: () => void }) {
  const [contacts, setContacts] = useState<Contact[]>([])
  const [activities, setActivities] = useState<Activity[]>([])
  const [reminders, setReminders] = useState<Reminder[]>([])
  const [opportunities, setOpportunities] = useState<Opportunity[]>([])
  const [selectedID, setSelectedID] = useState('')
  const [creating, setCreating] = useState(openNew)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [actionError, setActionError] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError('')
    Promise.all([
      api<{ contacts: Contact[] }>('/api/v1/contacts'),
      api<{ activities: Activity[] }>('/api/v1/activities'),
      api<{ reminders: Reminder[] }>('/api/v1/reminders'),
      api<{ opportunities: Opportunity[] }>('/api/v1/opportunities'),
    ]).then(([contactResponse, activityResponse, reminderResponse, opportunityResponse]) => {
      setContacts(contactResponse.contacts)
      setActivities(activityResponse.activities)
      setReminders(reminderResponse.reminders)
      setOpportunities(opportunityResponse.opportunities)
    }).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])

  useEffect(load, [load])
  useEffect(() => { if (openNew) setCreating(true) }, [openNew])

  const selected = contacts.find((contact) => contact.id === selectedID)

  async function createContact(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    const form = new FormData(event.currentTarget)
    try {
      const contact = await api<Contact>('/api/v1/contacts', { method: 'POST', body: JSON.stringify({ name: form.get('name'), company: form.get('company'), email: form.get('email'), phone: form.get('phone'), status: form.get('status') }) })
      setContacts((current) => [contact, ...current])
      setSelectedID(contact.id)
      setCreating(false)
      clearNew()
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Could not save contact')
    } finally {
      setSaving(false)
    }
  }

  async function updateStatus(contact: Contact, status: Contact['status']) {
    setActionError('')
    try {
      const updated = await api<Contact>(`/api/v1/contacts/${contact.id}`, { method: 'PATCH', body: JSON.stringify({ status }) })
      setContacts((current) => current.map((item) => item.id === updated.id ? updated : item))
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : 'Could not update contact')
    }
  }

  async function addActivity(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    const form = event.currentTarget
    const data = new FormData(form)
    setActionError('')
    try {
      const created = await api<Activity>('/api/v1/activities', { method: 'POST', body: JSON.stringify({ contactId: selected.id, kind: data.get('kind'), body: data.get('body') }) })
      setActivities((current) => [created, ...current])
      form.reset()
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : 'Could not add activity')
    }
  }

  async function addReminder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    const form = event.currentTarget
    const data = new FormData(form)
    setActionError('')
    try {
      const created = await api<Reminder>('/api/v1/reminders', { method: 'POST', body: JSON.stringify({ contactId: selected.id, title: data.get('title'), dueAt: new Date(String(data.get('dueAt'))).toISOString() }) })
      setReminders((current) => [...current, created])
      form.reset()
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : 'Could not add reminder')
    }
  }

  if (loading) return <LoadingState label="Loading your people" />
  if (error) return <ErrorState message={error} retry={load} />
  if (selected) return <ContactAccount contact={selected} activities={activities.filter((item) => item.contactId === selected.id)} reminders={reminders.filter((item) => item.contactId === selected.id)} opportunities={opportunities.filter((item) => item.contactId === selected.id)} actionError={actionError} onBack={() => setSelectedID('')} onStatus={updateStatus} onActivity={addActivity} onReminder={addReminder} />

  return <>
    <Page eyebrow="Relationships" title="Contacts" detail="Every lead, prospect, and customer in one human-friendly place." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Add contact</button>}>
      {contacts.length ? <div className="record-grid">{contacts.map((contact) => <button className="record-card" key={contact.id} onClick={() => setSelectedID(contact.id)}><span className="record-avatar">{initials(contact.name)}</span><span className="record-main"><strong>{contact.name}</strong><small>{contact.company || contact.email || 'No company yet'}</small></span><span className={`status-badge ${contact.status}`}>{contact.status}</span></button>)}</div> : <EmptyState title="No people yet" detail="Add the first person you want to follow up with." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Add your first contact</button>} />}
    </Page>
    {creating && <Modal eyebrow="Relationships" title="Add a contact" onClose={() => { setCreating(false); clearNew() }}><form onSubmit={createContact}><div className="field-grid"><label>Full name<input name="name" maxLength={160} required autoFocus /></label><label>Company<input name="company" maxLength={160} /></label><label>Email<input name="email" type="email" /></label><label>Phone<input name="phone" type="tel" /></label></div><label>Status<select name="status" defaultValue="lead"><option value="lead">Lead</option><option value="prospect">Prospect</option><option value="customer">Customer</option></select></label>{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => { setCreating(false); clearNew() }}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? 'Saving...' : 'Save contact'}</button></div></form></Modal>}
  </>
}

function ContactAccount({ contact, activities, reminders, opportunities, actionError, onBack, onStatus, onActivity, onReminder }: { contact: Contact; activities: Activity[]; reminders: Reminder[]; opportunities: Opportunity[]; actionError: string; onBack: () => void; onStatus: (contact: Contact, status: Contact['status']) => void; onActivity: (event: FormEvent<HTMLFormElement>) => void; onReminder: (event: FormEvent<HTMLFormElement>) => void }) {
  const openReminders = reminders.filter((item) => !item.completed)
  const latest = useMemo(() => [...activities].sort((a, b) => new Date(b.occurredAt).getTime() - new Date(a.occurredAt).getTime()), [activities])
  return <div className="page">
    <button className="back-button" onClick={onBack}><ArrowLeft size={16} /> All contacts</button>
    <header className="account-hero"><span className="record-avatar large">{initials(contact.name)}</span><div><p className="eyebrow">Account</p><h1>{contact.name}</h1><p className="subhead">{contact.company || 'Independent contact'}</p></div><label className="inline-select">Relationship<select aria-label="Relationship status" value={contact.status} onChange={(event) => onStatus(contact, event.target.value as Contact['status'])}><option value="lead">Lead</option><option value="prospect">Prospect</option><option value="customer">Customer</option></select></label></header>
    {actionError && <p className="form-error action-error" role="alert">{actionError}</p>}
    <section className="account-facts">{contact.company && <span><Building2 size={16} /><small>Company</small><strong>{contact.company}</strong></span>}{contact.email && <a href={`mailto:${contact.email}`}><Mail size={16} /><small>Email</small><strong>{contact.email}</strong></a>}{contact.phone && <a href={`tel:${contact.phone}`}><Phone size={16} /><small>Phone</small><strong>{contact.phone}</strong></a>}</section>
    <div className="account-grid">
      <section className="panel"><div className="panel-heading"><div><p className="eyebrow">History</p><h2>Activity and notes</h2></div></div><form className="quick-entry" onSubmit={onActivity}><select name="kind" aria-label="Activity type"><option value="note">Note</option><option value="call">Call</option><option value="email">Email</option><option value="meeting">Meeting</option></select><textarea name="body" aria-label="Activity note" placeholder="What happened?" maxLength={4000} required /><button className="primary-button"><MessageSquarePlus size={16} /> Add to timeline</button></form><div className="timeline">{latest.length ? latest.map((item) => <div className="timeline-item" key={item.id}><span className="timeline-dot" /><div><strong>{item.kind}</strong><p>{item.body}</p><time>{shortDate(item.occurredAt)}</time></div></div>) : <p className="quiet-copy">No notes yet. Capture the first conversation above.</p>}</div></section>
      <div className="account-side"><section className="panel"><div className="panel-heading"><div><p className="eyebrow">Next</p><h2>Follow-ups</h2></div></div><form className="stack-form" onSubmit={onReminder}><label>What needs to happen?<input name="title" maxLength={160} required /></label><label>When?<input name="dueAt" type="datetime-local" required /></label><button className="secondary-button"><CalendarPlus size={16} /> Add reminder</button></form>{openReminders.length ? <ul className="simple-list">{openReminders.map((item) => <li key={item.id}><strong>{item.title}</strong><small>{shortDate(item.dueAt)}</small></li>)}</ul> : <p className="quiet-copy">Nothing waiting on you.</p>}</section><section className="panel"><div className="panel-heading"><div><p className="eyebrow">Pipeline</p><h2>Opportunities</h2></div></div>{opportunities.length ? <ul className="simple-list">{opportunities.map((item) => <li key={item.id}><strong>{item.name}</strong><small>{item.stage}</small></li>)}</ul> : <p className="quiet-copy">No opportunities linked yet.</p>}</section></div>
    </div>
  </div>
}

function initials(name: string) {
  return name.split(' ').map((part) => part[0]).join('').slice(0, 2).toUpperCase()
}
