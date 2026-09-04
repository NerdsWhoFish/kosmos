import { FormEvent, useCallback, useEffect, useState } from 'react'
import { KeyRound, ShieldCheck, SlidersHorizontal, UserRound, Users } from 'lucide-react'
import { api, AuditEntry, GoogleStatus, Member, PipelineStage, shortDate, User } from '../api'
import { Page } from '../components/Page'
import { ErrorState, LoadingState } from '../components/States'

export function Settings({ user }: { user: User }) {
  const [members, setMembers] = useState<Member[]>([])
  const [stages, setStages] = useState<PipelineStage[]>([])
  const [audit, setAudit] = useState<AuditEntry[]>([])
  const [google, setGoogle] = useState<GoogleStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(() => {
    Promise.all([
      api<{ members: Member[] }>('/api/v1/members'),
      api<{ stages: PipelineStage[] }>('/api/v1/pipeline-stages'),
      api<{ entries: AuditEntry[] }>('/api/v1/audit'),
      api<GoogleStatus>('/api/v1/integrations/google'),
    ]).then(([team, pipeline, history, connection]) => {
      setMembers(team.members)
      setStages(pipeline.stages)
      setAudit(history.entries)
      setGoogle(connection)
      setError('')
    }).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  async function changeRole(member: Member, role: Member['role']) {
    try {
      await api(`/api/v1/members/${member.id}`, { method: 'PATCH', body: JSON.stringify({ role, status: member.status }) })
      load()
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Could not update role') }
  }

  async function createStage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const formElement = event.currentTarget
    const form = new FormData(formElement)
    try {
      await api('/api/v1/pipeline-stages', { method: 'POST', body: JSON.stringify({ name: form.get('name'), position: Number(form.get('position')), probability: Number(form.get('probability')), closed: form.get('closed') === 'on', won: form.get('won') === 'on' }) })
      formElement.reset()
      load()
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Could not create stage') }
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} retry={load} />

  return <Page eyebrow="Workspace" title="Settings" detail="People, permissions, integrations, and the audit trail without an admin maze.">
    <div className="settings-grid"><section className="panel setting-card"><span className="setting-icon"><UserRound size={20} /></span><div><h2>Your Google account</h2><p>{user.name}</p><small>{user.email}</small></div></section><section className="panel setting-card"><span className="setting-icon"><ShieldCheck size={20} /></span><div><h2>Access policy</h2><p>Approved domains only</p><small>Every request rechecks the verified Google identity and active membership.</small></div></section><section className="panel setting-card"><span className="setting-icon"><KeyRound size={20} /></span><div><h2>Password and MFA</h2><p>Managed by Google</p><small>Kosmos never stores or resets your password.</small></div></section></div>
    <section className="split-grid lower-grid">
      <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Team</p><h2>Members and roles</h2></div><Users size={20} /></div><div className="table-list">{members.map((member) => <article className="member-row" key={member.id}><span className="record-avatar">{member.name.split(' ').map((part) => part[0]).join('').slice(0, 2)}</span><span><strong>{member.name}</strong><small>{member.email}</small></span><select aria-label={`Role for ${member.name}`} value={member.role} onChange={(event) => changeRole(member, event.target.value as Member['role'])}><option value="owner">Owner</option><option value="admin">Admin</option><option value="member">Member</option><option value="viewer">Viewer</option></select></article>)}</div></div>
      <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Google Workspace</p><h2>{google?.connected ? 'Connected' : 'Not connected'}</h2></div><SlidersHorizontal size={20} /></div><p className="muted-copy">Gmail compose, relevant message metadata, and read-only Google Sheets access are granted separately from login.</p>{google?.connected ? <p className="inline-notice"><span className="security-dot" /> {google.connection?.googleEmail}</p> : <a className="primary-button" href={google?.connectUrl ?? '/auth/connect/workspace'}>Connect Google Workspace</a>}</div>
    </section>
    <section className="panel lower-panel"><div className="panel-heading"><div><p className="eyebrow">Pipeline</p><h2>Your stages</h2></div></div><div className="stage-strip">{stages.map((stage) => <span key={stage.id}><strong>{stage.name}</strong><small>{stage.probability}%</small></span>)}</div><form className="inline-form" onSubmit={createStage}><label>Stage name<input name="name" maxLength={80} required /></label><label>Position<input name="position" type="number" min="0" defaultValue={stages.length} required /></label><label>Probability<input name="probability" type="number" min="0" max="100" defaultValue="50" required /></label><label className="check-label"><input name="closed" type="checkbox" /> Closed</label><label className="check-label"><input name="won" type="checkbox" /> Won</label><button className="secondary-button">Add stage</button></form></section>
    <section className="panel lower-panel"><div className="panel-heading"><div><p className="eyebrow">Audit history</p><h2>Who changed what</h2></div></div><div className="table-list">{audit.length ? audit.slice(0, 30).map((entry) => <article className="audit-row" key={entry.id}><span><strong>{entry.summary}</strong><small>{entry.actor} · {entry.action}</small></span><time>{shortDate(entry.createdAt)}</time></article>) : <p className="muted-copy">Security-sensitive and external actions will appear here.</p>}</div></section>
  </Page>
}
