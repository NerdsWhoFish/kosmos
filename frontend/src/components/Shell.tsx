import { FormEvent, ReactNode, useState } from 'react'
import { Bell, CheckCircle2, CircleDollarSign, FileText, LayoutGrid, Search, Settings2, Users, Waves } from 'lucide-react'
import type { User } from '../api'

type Navigate = (path: string) => void

const navigation = [
  { path: '/', label: 'Overview', icon: LayoutGrid },
  { path: '/contacts', label: 'Contacts', icon: Users },
  { path: '/opportunities', label: 'Opportunities', icon: CircleDollarSign },
  { path: '/documents', label: 'Documents', icon: FileText },
  { path: '/costs', label: 'Costs', icon: CheckCircle2 },
]

export function Shell({ user, path, navigate, logout, children }: { user: User; path: string; navigate: Navigate; logout: () => void; children: ReactNode }) {
  const [query, setQuery] = useState('')
  function search(event: FormEvent) {
    event.preventDefault()
    const value = query.trim()
    if (value) navigate(`/search?q=${encodeURIComponent(value)}`)
  }

  return <div className="app-shell">
    <aside className="sidebar">
      <a className="brand" href="/" onClick={(event) => { event.preventDefault(); navigate('/') }}><span className="brand-mark"><Waves size={20} /></span><span className="brand-name">Kosmos</span></a>
      <p className="eyebrow sidebar-label">Your workspace</p>
      <nav className="nav-list" aria-label="Workspace">{navigation.map(({ path: target, label, icon: Icon }) => <a key={target} aria-label={label} className={`nav-item ${path === target ? 'active' : ''}`} href={target} onClick={(event) => { event.preventDefault(); navigate(target) }}><Icon size={18} /><span className="nav-label">{label}</span></a>)}</nav>
      <div className="sidebar-spacer" />
      <a aria-label="Settings" className={`nav-item ${path === '/settings' ? 'active' : ''}`} href="/settings" onClick={(event) => { event.preventDefault(); navigate('/settings') }}><Settings2 size={18} /><span className="nav-label">Settings</span></a>
      <div className="user-chip"><span className="avatar">{initials(user.name)}</span><span><strong>{user.name}</strong><small>{user.email}</small></span></div>
    </aside>
    <main className="main-content">
      <header className="topbar">
        <form className="search" role="search" onSubmit={search}><Search size={18} /><input aria-label="Search Kosmos" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search people, documents, and costs" /></form>
        <div className="top-actions"><button className="icon-button" aria-label="Notifications and follow-ups" onClick={() => navigate('/activity')}><Bell size={19} /></button><button className="account-button" onClick={logout}><span className="avatar">{initials(user.name)}</span><span>Sign out</span></button></div>
      </header>
      <div className="content-wrap">{children}</div>
    </main>
  </div>
}

export function PublicLogin() {
  return <main className="public-login">
    <div className="public-atmosphere" aria-hidden="true"><span /><span /><span /></div>
    <section className="public-copy">
      <div className="public-brand"><span className="brand-mark"><Waves size={22} /></span><span>Kosmos</span></div>
      <p className="eyebrow">Nerds Who Fish workspace</p>
      <h1>Your business,<br /><em>without the busywork.</em></h1>
      <p className="public-lede">One calm place for people, opportunities, notes, documents, follow-ups, and the money keeping the lights on.</p>
      <a className="public-signin" href="/auth/login"><span>Continue with Google</span><span aria-hidden="true">→</span></a>
      <p className="public-security"><span className="security-dot" />Access is limited to approved company Google accounts.</p>
    </section>
    <aside className="public-orbit" aria-label="Kosmos organizes your business">
      <div className="orbit-core"><Waves size={34} /><strong>Your work,<br />in orbit.</strong></div>
      <span className="orbit-chip orbit-people">People</span>
      <span className="orbit-chip orbit-work">Next steps</span>
      <span className="orbit-chip orbit-knowledge">Knowledge</span>
      <span className="orbit-chip orbit-money">Costs</span>
    </aside>
  </main>
}

function initials(name: string) {
  return name.split(' ').map((part) => part[0]).join('').slice(0, 2).toUpperCase()
}
