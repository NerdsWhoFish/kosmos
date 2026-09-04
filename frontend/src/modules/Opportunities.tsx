import { FormEvent, useCallback, useEffect, useState } from 'react'
import { CircleDollarSign, Plus } from 'lucide-react'
import { api, Contact, money, Opportunity, shortDate } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

const stages: Opportunity['stage'][] = ['new', 'qualified', 'proposal', 'won', 'lost']

export function Opportunities() {
  const [items, setItems] = useState<Opportunity[]>([])
  const [contacts, setContacts] = useState<Contact[]>([])
  const [creating, setCreating] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError('')
    Promise.all([api<{ opportunities: Opportunity[] }>('/api/v1/opportunities'), api<{ contacts: Contact[] }>('/api/v1/contacts')])
      .then(([opportunities, people]) => { setItems(opportunities.opportunities); setContacts(people.contacts) })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    const form = new FormData(event.currentTarget)
    try {
      const amount = Math.round(Number(form.get('amount')) * 100)
      const created = await api<Opportunity>('/api/v1/opportunities', { method: 'POST', body: JSON.stringify({ name: form.get('name'), contactId: form.get('contactId'), amountCents: amount, stage: form.get('stage'), nextStep: form.get('nextStep'), closeDate: form.get('closeDate') }) })
      setItems((current) => [created, ...current])
      setCreating(false)
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Could not save opportunity')
    } finally {
      setSaving(false)
    }
  }

  async function move(item: Opportunity, stage: Opportunity['stage']) {
    try {
      const updated = await api<Opportunity>(`/api/v1/opportunities/${item.id}`, { method: 'PATCH', body: JSON.stringify({ stage }) })
      setItems((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Could not move opportunity')
    }
  }

  if (loading) return <LoadingState label="Loading your pipeline" />
  if (error) return <ErrorState message={error} retry={load} />

  return <>
    <Page eyebrow="Pipeline" title="Opportunities" detail="Know what is moving, what is stuck, and what each win is worth." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Add opportunity</button>}>
      {items.length ? <div className="pipeline-board">{stages.map((stage) => <section className={`pipeline-column ${stage}`} key={stage}><header><span>{stage}</span><strong>{items.filter((item) => item.stage === stage).length}</strong></header><div>{items.filter((item) => item.stage === stage).map((item) => <article className="opportunity-card" key={item.id}><span className="opportunity-value"><CircleDollarSign size={15} />{money(item.amountCents)}</span><h3>{item.name}</h3><p>{contactName(contacts, item.contactId)}</p>{item.nextStep && <small>Next: {item.nextStep}</small>}{item.closeDate && <time>Close {shortDate(item.closeDate + 'T12:00:00Z')}</time>}<label>Move to<select aria-label={`Stage for ${item.name}`} value={item.stage} onChange={(event) => move(item, event.target.value as Opportunity['stage'])}>{stages.map((choice) => <option key={choice} value={choice}>{choice}</option>)}</select></label></article>)}</div></section>)}</div> : <EmptyState title="No opportunities yet" detail="Create one when a conversation has a real next step and potential value." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Add your first opportunity</button>} />}
    </Page>
    {creating && <Modal eyebrow="Pipeline" title="Add an opportunity" onClose={() => setCreating(false)}><form onSubmit={create}><label>Opportunity name<input name="name" maxLength={160} required autoFocus /></label><div className="field-grid"><label>Contact<select name="contactId" defaultValue=""><option value="">No contact yet</option>{contacts.map((contact) => <option key={contact.id} value={contact.id}>{contact.name}</option>)}</select></label><label>Value<input name="amount" type="number" inputMode="decimal" min="0" step="0.01" defaultValue="0" required /></label><label>Stage<select name="stage" defaultValue="new">{stages.map((stage) => <option key={stage} value={stage}>{stage}</option>)}</select></label><label>Target close<input name="closeDate" type="date" /></label></div><label>Next step<input name="nextStep" maxLength={240} placeholder="Send the proposal" /></label>{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? 'Saving...' : 'Save opportunity'}</button></div></form></Modal>}
  </>
}

function contactName(contacts: Contact[], id: string) {
  return contacts.find((contact) => contact.id === id)?.name || 'Unlinked opportunity'
}
