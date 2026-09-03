import { FormEvent, useEffect, useState } from 'react'
import { ArrowUpRight, Bell, BookOpen, CalendarDays, CheckCircle2, CircleDollarSign, FileText, Globe2, LayoutGrid, Mail, Plus, Search, Settings2, Users, Waves, X } from 'lucide-react'

type Button = { id: string; label: string; description: string; href: string; icon: string }
type Notification = { id: string; title: string; summary: string; kind: string; createdAt: string; href: string }
type Landing = { buttons: Button[]; notifications: Notification[] }
type User = { email: string; name: string; picture?: string }

const emptyLanding: Landing = { buttons: [], notifications: [] }
const iconMap: Record<string, typeof Globe2> = { globe: Globe2, calendar: CalendarDays, users: Users }

export function App() {
  const [landing, setLanding] = useState<Landing>(emptyLanding)
  const [user, setUser] = useState<User | null>(null)
  const [shortcutOpen, setShortcutOpen] = useState(false)
  const [shortcutError, setShortcutError] = useState('')
  const [savingShortcut, setSavingShortcut] = useState(false)

  useEffect(() => {
    fetch('/api/v1/me')
      .then((response) => response.ok ? response.json() : null)
      .then((currentUser: User | null) => {
        setUser(currentUser)
        if (!currentUser) return
        return fetch('/api/v1/landing')
          .then((response) => response.ok ? response.json() : Promise.reject(new Error('landing request failed')))
          .then(setLanding)
      })
      .catch(() => setLanding(emptyLanding))
  }, [])

  async function createShortcut(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSavingShortcut(true)
    setShortcutError('')
    const form = new FormData(event.currentTarget)
    try {
      const response = await fetch('/api/v1/landing/buttons', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          label: form.get('label'),
          description: form.get('description'),
          href: form.get('href'),
        }),
      })
      const result = await response.json()
      if (!response.ok) throw new Error(result.error ?? 'Could not save shortcut')
      setLanding((current) => ({ ...current, buttons: [...current.buttons, result] }))
      setShortcutOpen(false)
    } catch (error) {
      setShortcutError(error instanceof Error ? error.message : 'Could not save shortcut')
    } finally {
      setSavingShortcut(false)
    }
  }

  const firstName = user?.name?.split(' ')[0] ?? 'there'
  const today = new Intl.DateTimeFormat('en-US', { weekday: 'long', month: 'long', day: 'numeric' }).format(new Date())

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark"><Waves size={20} /></span><span className="brand-name">Kosmos</span></div>
      <p className="eyebrow sidebar-label">Your workspace</p>
      <nav className="nav-list" aria-label="Workspace">
        <a aria-label="Overview" className="nav-item active" href="#home"><LayoutGrid size={18} /><span className="nav-label">Overview</span></a>
        <a aria-label="Contacts" className="nav-item" href="#contacts"><Users size={18} /><span className="nav-label">Contacts</span><span className="nav-count">12</span></a>
        <a aria-label="Opportunities" className="nav-item" href="#opportunities"><CircleDollarSign size={18} /><span className="nav-label">Opportunities</span></a>
        <a aria-label="Documents" className="nav-item" href="#documents"><FileText size={18} /><span className="nav-label">Documents</span></a>
        <a aria-label="Costs" className="nav-item" href="#costs"><CheckCircle2 size={18} /><span className="nav-label">Costs</span></a>
      </nav>
      <div className="sidebar-spacer" />
      <a aria-label="Settings" className="nav-item" href="#settings"><Settings2 size={18} /><span className="nav-label">Settings</span></a>
      <div className="user-chip"><span className="avatar">{user?.name?.split(' ').map((part) => part[0]).join('').slice(0, 2) ?? '??'}</span><span><strong>{user?.name ?? 'Not signed in'}</strong><small>{user?.email ?? 'Google account required'}</small></span><ArrowUpRight size={15} /></div>
    </aside>
    <main className="main-content" id="home">
      <header className="topbar"><div className="search"><Search size={18} /><input aria-label="Search Kosmos" placeholder="Search anything..." /></div><div className="top-actions"><button className="icon-button" aria-label="Notifications"><Bell size={19} /><span className="notification-dot" /></button>{user ? <button className="primary-button" onClick={() => { fetch('/auth/logout', { method: 'POST' }).then(() => { setUser(null); setLanding(emptyLanding) }) }}><CheckCircle2 size={17} /> Sign out</button> : <a className="primary-button" href="/auth/login"><Plus size={17} /> Sign in with Google</a>}</div></header>
      <div className="content-wrap">
        <section className="welcome-row"><div><p className="eyebrow">{today}</p><h1>Good morning, {firstName}.</h1><p className="subhead">Here’s the pulse of your business.</p></div><div className="weather-card"><span className="weather-icon">☀</span><span><strong>78°</strong><small>Reynoldsburg, OH</small></span></div></section>
        <section className="stats-row"><Stat label="Open opportunities" value="6" detail="$18,400 potential" tone="blue" /><Stat label="Follow-ups due" value="3" detail="2 need attention today" tone="gold" /><Stat label="This month’s costs" value="$842" detail="4 recurring expenses" tone="green" /></section>
        <section className="dashboard-grid">
          <div className="panel quick-panel"><div className="panel-heading"><div><p className="eyebrow">Your shortcuts</p><h2>Landing zone</h2></div>{user ? <button className="text-button" onClick={() => setShortcutOpen(true)}><Plus size={15} /> Customize</button> : <a className="text-button" href="/auth/login"><Plus size={15} /> Sign in to customize</a>}</div><div className="shortcut-grid">{landing.buttons.map((button) => { const Icon = iconMap[button.icon] ?? Globe2; return <a className="shortcut-card" href={button.href} key={button.id}><span className="shortcut-icon"><Icon size={20} /></span><span><strong>{button.label}</strong><small>{button.description}</small></span><ArrowUpRight className="shortcut-arrow" size={17} /></a> })}{user ? <button className="shortcut-card add-card" onClick={() => setShortcutOpen(true)}><span className="shortcut-icon muted"><Plus size={20} /></span><span><strong>Add a shortcut</strong><small>Put your most-used link here.</small></span></button> : <a className="shortcut-card add-card" href="/auth/login"><span className="shortcut-icon muted"><Plus size={20} /></span><span><strong>Sign in to add shortcuts</strong><small>Your links are private to your Google account.</small></span></a>}</div></div>
          <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Stay in the loop</p><h2>Recent activity</h2></div><button className="text-button">View all <ArrowUpRight size={15} /></button></div><div className="activity-list">{landing.notifications.map((item) => <div className="activity-item" key={item.id}><span className="activity-icon"><Mail size={16} /></span><span><strong>{item.title}</strong><small>{item.summary}</small><time>Just now</time></span></div>)}<div className="activity-item"><span className="activity-icon lavender"><BookOpen size={16} /></span><span><strong>Make this space yours</strong><small>Add a document, note, or reminder to get started.</small><time>Whenever you’re ready</time></span></div></div></div>
        </section>
        <section className="tip-banner"><span className="tip-icon">✦</span><span><strong>Start with one small thing.</strong><small>Kosmos gets more useful as you add your people, notes, and next steps.</small></span><button className="banner-button">Add a contact <ArrowUpRight size={15} /></button></section>
      </div>
    </main>
    {shortcutOpen && <div className="modal-backdrop"><section className="modal" role="dialog" aria-modal="true" aria-labelledby="shortcut-title"><div className="modal-heading"><div><p className="eyebrow">Landing zone</p><h2 id="shortcut-title">Add a shortcut</h2></div><button className="icon-button" aria-label="Close shortcut form" onClick={() => setShortcutOpen(false)}><X size={20} /></button></div><form onSubmit={createShortcut}><label>Button name<input name="label" maxLength={80} required autoFocus /></label><label>Link<input name="href" inputMode="url" placeholder="https://example.com" required /></label><label>Description<textarea name="description" maxLength={180} rows={3} /></label>{shortcutError && <p className="form-error" role="alert">{shortcutError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setShortcutOpen(false)}>Cancel</button><button className="primary-button" disabled={savingShortcut}>{savingShortcut ? 'Saving...' : 'Save shortcut'}</button></div></form></section></div>}
  </div>
}

function Stat({ label, value, detail, tone }: { label: string; value: string; detail: string; tone: string }) { return <div className={`stat-card ${tone}`}><span className="stat-label">{label}</span><strong className="stat-value">{value}</strong><small>{detail}</small></div> }
