import { KeyRound, ShieldCheck, UserRound } from 'lucide-react'
import type { User } from '../api'
import { Page } from '../components/Page'

export function Settings({ user }: { user: User }) {
  return <Page eyebrow="Workspace" title="Settings" detail="The important account and security details, without an admin maze.">
    <div className="settings-grid"><section className="panel setting-card"><span className="setting-icon"><UserRound size={20} /></span><div><h2>Your Google account</h2><p>{user.name}</p><small>{user.email}</small></div></section><section className="panel setting-card"><span className="setting-icon"><ShieldCheck size={20} /></span><div><h2>Access policy</h2><p>Company accounts only</p><small>Access is checked by verified Google email domain on every session.</small></div></section><section className="panel setting-card"><span className="setting-icon"><KeyRound size={20} /></span><div><h2>Password and MFA</h2><p>Managed by Google</p><small>Kosmos never stores or resets your password.</small></div></section></div>
  </Page>
}
