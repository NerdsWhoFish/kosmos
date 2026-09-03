import { useEffect, useState } from 'react'
import { ArrowUpRight, Bell, BookOpen, CalendarDays, CheckCircle2, CircleDollarSign, FileText, Globe2, LayoutGrid, Mail, Plus, Search, Settings2, Users, Waves } from 'lucide-react'

type Button = { id: string; label: string; description: string; href: string; icon: string }
type Notification = { id: string; title: string; summary: string; kind: string; createdAt: string; href: string }
type Landing = { buttons: Button[]; notifications: Notification[] }

const iconMap: Record<string, typeof Globe2> = { globe: Globe2, calendar: CalendarDays, users: Users }

export function App() {
  const [landing, setLanding] = useState<Landing>({ buttons: [], notifications: [] })

  useEffect(() => {
    fetch('/api/v1/landing')
      .then((response) => response.ok ? response.json() : Promise.reject(new Error('landing request failed')))
      .then(setLanding)
      .catch(() => setLanding({ buttons: [
        { id: 'website', label: 'Open website', description: 'Jump straight to the public business site.', href: 'https://www.nerdswhofish.com', icon: 'globe' },
        { id: 'bookings', label: 'Bookings', description: 'Manage meetings and availability.', href: 'https://book.nerdswhofish.com', icon: 'calendar' },
        { id: 'contacts', label: 'Contacts', description: 'Keep every relationship in one place.', href: '/contacts', icon: 'users' },
      ], notifications: [{ id: 'welcome', title: 'Kosmos is ready', summary: 'Your business home base is ready to customize.', kind: 'system', createdAt: new Date().toISOString(), href: '/docs/getting-started' }] }))
  }, [])

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark"><Waves size={20} /></span><span>Kosmos</span></div>
      <p className="eyebrow sidebar-label">Your workspace</p>
      <nav className="nav-list">
        <a className="nav-item active" href="#home"><LayoutGrid size={18} /> Overview</a>
        <a className="nav-item" href="#contacts"><Users size={18} /> Contacts <span className="nav-count">12</span></a>
        <a className="nav-item" href="#opportunities"><CircleDollarSign size={18} /> Opportunities</a>
        <a className="nav-item" href="#documents"><FileText size={18} /> Documents</a>
        <a className="nav-item" href="#costs"><CheckCircle2 size={18} /> Costs</a>
      </nav>
      <div className="sidebar-spacer" />
      <a className="nav-item" href="#settings"><Settings2 size={18} /> Settings</a>
      <div className="user-chip"><span className="avatar">JS</span><span><strong>Joey Stout</strong><small>Owner</small></span><ArrowUpRight size={15} /></div>
    </aside>
    <main className="main-content">
      <header className="topbar"><div className="search"><Search size={18} /><input aria-label="Search Kosmos" placeholder="Search anything..." /></div><div className="top-actions"><button className="icon-button" aria-label="Notifications"><Bell size={19} /><span className="notification-dot" /></button><button className="primary-button"><Plus size={17} /> Add something</button></div></header>
      <div className="content-wrap">
        <section className="welcome-row"><div><p className="eyebrow">Wednesday, September 3</p><h1>Good morning, Joey.</h1><p className="subhead">Here’s the pulse of your business.</p></div><div className="weather-card"><span className="weather-icon">☀</span><span><strong>78°</strong><small>Reynoldsburg, OH</small></span></div></section>
        <section className="stats-row"><Stat label="Open opportunities" value="6" detail="$18,400 potential" tone="blue" /><Stat label="Follow-ups due" value="3" detail="2 need attention today" tone="gold" /><Stat label="This month’s costs" value="$842" detail="4 recurring expenses" tone="green" /></section>
        <section className="dashboard-grid">
          <div className="panel quick-panel"><div className="panel-heading"><div><p className="eyebrow">Your shortcuts</p><h2>Landing zone</h2></div><button className="text-button"><Plus size={15} /> Customize</button></div><div className="shortcut-grid">{landing.buttons.map((button) => { const Icon = iconMap[button.icon] ?? Globe2; return <a className="shortcut-card" href={button.href} key={button.id}><span className="shortcut-icon"><Icon size={20} /></span><span><strong>{button.label}</strong><small>{button.description}</small></span><ArrowUpRight className="shortcut-arrow" size={17} /></a> })}<button className="shortcut-card add-card"><span className="shortcut-icon muted"><Plus size={20} /></span><span><strong>Add a shortcut</strong><small>Put your most-used link here.</small></span></button></div></div>
          <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Stay in the loop</p><h2>Recent activity</h2></div><button className="text-button">View all <ArrowUpRight size={15} /></button></div><div className="activity-list">{landing.notifications.map((item) => <div className="activity-item" key={item.id}><span className="activity-icon"><Mail size={16} /></span><span><strong>{item.title}</strong><small>{item.summary}</small><time>Just now</time></span></div>)}<div className="activity-item"><span className="activity-icon lavender"><BookOpen size={16} /></span><span><strong>Make this space yours</strong><small>Add a document, note, or reminder to get started.</small><time>Whenever you’re ready</time></span></div></div></div>
        </section>
        <section className="tip-banner"><span className="tip-icon">✦</span><span><strong>Start with one small thing.</strong><small>Kosmos gets more useful as you add your people, notes, and next steps.</small></span><button className="banner-button">Add a contact <ArrowUpRight size={15} /></button></section>
      </div>
    </main>
  </div>
}

function Stat({ label, value, detail, tone }: { label: string; value: string; detail: string; tone: string }) { return <div className={`stat-card ${tone}`}><span className="stat-label">{label}</span><strong className="stat-value">{value}</strong><small>{detail}</small></div> }
