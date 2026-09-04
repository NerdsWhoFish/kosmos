import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Building2, ExternalLink, Plus } from 'lucide-react'
import { Account, api, Contact, money, Opportunity } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

type AccountDetail = { account: Account; contacts: Contact[]; opportunities: Opportunity[] }

export function Accounts({ initialID, navigate }: { initialID: string; navigate: (path: string) => void }) {
  const [items, setItems] = useState<Account[]>([])
  const [selected, setSelected] = useState<AccountDetail | null>(null)
  const [creating, setCreating] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(() => api<{ accounts: Account[] }>('/api/v1/accounts').then((response) => { setItems(response.accounts); setError('') }).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false)), [])
  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (!initialID) {
      setSelected(null)
      return
    }
    api<AccountDetail>(`/api/v1/accounts/${initialID}`).then(setSelected).catch((reason: Error) => setError(reason.message))
  }, [initialID])

  async function open(account: Account) {
    navigate(`/accounts/${account.id}`)
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const created = await api<Account>('/api/v1/accounts', { method: 'POST', body: JSON.stringify({ name: form.get('name'), website: form.get('website'), billingEmail: form.get('billingEmail'), status: form.get('status'), notes: form.get('notes') }) })
    setItems((current) => [created, ...current])
    setCreating(false)
    await open(created)
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} retry={load} />
  if (selected) return <div className="page"><button className="back-button" onClick={() => navigate('/accounts')}>← All accounts</button><header className="account-hero"><span className="record-avatar large"><Building2 size={26} /></span><div><p className="eyebrow">{selected.account.status}</p><h1>{selected.account.name}</h1><p className="subhead">{selected.account.billingEmail || 'No billing email yet'}</p></div>{selected.account.website && <a className="secondary-button" href={selected.account.website} target="_blank" rel="noreferrer">Website <ExternalLink size={15} /></a>}</header><section className="stats-row"><div className="stat-card blue"><span className="stat-label">Contacts</span><strong className="stat-value">{selected.contacts.length}</strong></div><div className="stat-card gold"><span className="stat-label">Open opportunities</span><strong className="stat-value">{selected.opportunities.filter((item) => !['won', 'lost'].includes(item.stage)).length}</strong></div><div className="stat-card green"><span className="stat-label">Pipeline</span><strong className="stat-value">{money(selected.opportunities.reduce((sum, item) => sum + item.amountCents, 0))}</strong></div></section><section className="split-grid"><div className="panel"><p className="eyebrow">People</p><h2>Account contacts</h2>{selected.contacts.length ? selected.contacts.map((contact) => <a className="record-row compact" href={`/contacts/${contact.id}`} key={contact.id}><strong>{contact.name}</strong><small>{contact.email}</small></a>) : <p className="muted-copy">Link a contact to this account when you add or edit them.</p>}</div><div className="panel"><p className="eyebrow">Pipeline</p><h2>Opportunities</h2>{selected.opportunities.length ? selected.opportunities.map((item) => <div className="record-row compact" key={item.id}><span><strong>{item.name}</strong><small>{item.stage}</small></span><strong>{money(item.amountCents)}</strong></div>) : <p className="muted-copy">No opportunities are linked yet.</p>}</div></section></div>

  return <><Page eyebrow="Relationships" title="Accounts" detail="See the whole customer relationship, not just one person." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Add account</button>}>{items.length ? <div className="record-grid">{items.map((account) => <button className="record-card" key={account.id} onClick={() => open(account)}><span className="record-avatar"><Building2 size={18} /></span><span className="record-main"><strong>{account.name}</strong><small>{account.billingEmail || account.website || 'No details yet'}</small></span><span className={`status-badge ${account.status}`}>{account.status}</span></button>)}</div> : <EmptyState title="No accounts yet" detail="Create an account for a prospect or customer with more than one contact." action={<button className="primary-button" onClick={() => setCreating(true)}>Add your first account</button>} />}</Page>{creating && <Modal eyebrow="Relationships" title="Add an account" onClose={() => setCreating(false)}><form onSubmit={create}><label>Business name<input name="name" required autoFocus /></label><div className="field-grid"><label>Website<input name="website" type="url" /></label><label>Billing email<input name="billingEmail" type="email" /></label><label>Status<select name="status" defaultValue="prospect"><option value="prospect">Prospect</option><option value="customer">Customer</option><option value="inactive">Inactive</option></select></label></div><label>Notes<textarea name="notes" rows={4} /></label><div className="form-actions"><button type="button" className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button">Save account</button></div></form></Modal>}</>
}
