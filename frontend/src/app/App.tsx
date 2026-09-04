import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { api, User } from '../api'
import { PublicLogin, Shell } from '../components/Shell'
import { LoadingState } from '../components/States'

const Activity = lazy(() => import('../modules/Activity').then((module) => ({ default: module.Activity })))
const Contacts = lazy(() => import('../modules/Contacts').then((module) => ({ default: module.Contacts })))
const Costs = lazy(() => import('../modules/Costs').then((module) => ({ default: module.Costs })))
const Documents = lazy(() => import('../modules/Documents').then((module) => ({ default: module.Documents })))
const Opportunities = lazy(() => import('../modules/Opportunities').then((module) => ({ default: module.Opportunities })))
const Overview = lazy(() => import('../modules/Overview').then((module) => ({ default: module.Overview })))
const SearchResults = lazy(() => import('../modules/SearchResults').then((module) => ({ default: module.SearchResults })))
const Settings = lazy(() => import('../modules/Settings').then((module) => ({ default: module.Settings })))

type LocationState = { path: string; search: string }

export function App() {
  const [user, setUser] = useState<User | null>(null)
  const [checkingSession, setCheckingSession] = useState(true)
  const [location, setLocation] = useState<LocationState>(() => currentLocation())

  useEffect(() => {
    fetch('/api/v1/me')
      .then((response) => response.ok ? response.json() : null)
      .then((current: User | null) => setUser(current))
      .catch(() => setUser(null))
      .finally(() => setCheckingSession(false))
  }, [])

  useEffect(() => {
    const sync = () => setLocation(currentLocation())
    window.addEventListener('popstate', sync)
    return () => window.removeEventListener('popstate', sync)
  }, [])

  const navigate = useCallback((target: string) => {
    window.history.pushState({}, '', target)
    setLocation(currentLocation())
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  async function logout() {
    await api('/auth/logout', { method: 'POST' })
    setUser(null)
    window.history.replaceState({}, '', '/')
    setLocation(currentLocation())
  }

  if (checkingSession) return <div className="session-loading"><LoadingState label="Opening Kosmos" /></div>
  if (!user) return <PublicLogin />

  return <Shell user={user} path={basePath(location.path)} navigate={navigate} logout={logout}>
    <Suspense fallback={<LoadingState />}><Route location={location} user={user} navigate={navigate} /></Suspense>
  </Shell>
}

function Route({ location, user, navigate }: { location: LocationState; user: User; navigate: (path: string) => void }) {
  const path = basePath(location.path)
  if (path === '/') return <Overview firstName={user.name.split(' ')[0] || 'there'} navigate={navigate} />
  if (path === '/contacts') return <Contacts openNew={new URLSearchParams(location.search).get('new') === '1'} clearNew={() => navigate('/contacts')} />
  if (path === '/opportunities') return <Opportunities />
  if (path === '/documents') return <Documents />
  if (path === '/costs') return <Costs />
  if (path === '/activity') return <Activity />
  if (path === '/settings') return <Settings user={user} />
  if (path === '/search') return <SearchResults query={new URLSearchParams(location.search).get('q') ?? ''} navigate={navigate} />
  return <Overview firstName={user.name.split(' ')[0] || 'there'} navigate={navigate} />
}

function currentLocation(): LocationState {
  return { path: window.location.pathname, search: window.location.search }
}

function basePath(path: string) {
  if (path.startsWith('/contacts/')) return '/contacts'
  return path
}
