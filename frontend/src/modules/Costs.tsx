import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Plus, ReceiptText, Repeat2 } from 'lucide-react'
import { api, Cost, money, shortDate } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function Costs() {
  const [items, setItems] = useState<Cost[]>([])
  const [creating, setCreating] = useState(false)
  const [recurring, setRecurring] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    api<{ costs: Cost[] }>('/api/v1/costs').then((response) => setItems(response.costs)).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await api<Cost>('/api/v1/costs', { method: 'POST', body: JSON.stringify({ vendor: form.get('vendor'), description: form.get('description'), amountCents: Math.round(Number(form.get('amount')) * 100), category: form.get('category'), incurredOn: form.get('incurredOn'), recurring, recurrence: recurring ? form.get('recurrence') : '', taxDeductible: form.get('taxDeductible') === 'on', notes: form.get('notes'), renewalDate: form.get('renewalDate'), paymentMethod: form.get('paymentMethod'), reviewState: form.get('reviewState') }) })
      setItems((current) => [created, ...current])
      setCreating(false)
      setRecurring(false)
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Could not save cost')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingState label="Loading business costs" />
  if (error) return <ErrorState message={error} retry={load} />

  const total = items.reduce((sum, item) => sum + item.amountCents, 0)
  return <>
    <Page eyebrow="Money" title="Business costs" detail="Track subscriptions, registrations, and every expense you will want at tax time." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Record a cost</button>}>
      {items.length ? <><section className="cost-summary"><span><small>Total recorded</small><strong>{money(total)}</strong></span><span><small>Recurring</small><strong>{items.filter((item) => item.recurring).length}</strong></span><span><small>Tax-deductible</small><strong>{items.filter((item) => item.taxDeductible).length}</strong></span></section><div className="record-table" role="table" aria-label="Business costs">{items.map((item) => <article className="cost-row" role="row" key={item.id}><span className="cost-icon">{item.recurring ? <Repeat2 size={18} /> : <ReceiptText size={18} />}</span><span className="record-main"><strong>{item.description}</strong><small>{[item.vendor, item.category, shortDate(item.incurredOn + 'T12:00:00Z')].filter(Boolean).join(' · ')}</small></span><span className="cost-flags">{item.taxDeductible && <small>Tax</small>}{item.recurring && <small>{item.recurrence}</small>}</span><strong className="cost-amount">{money(item.amountCents)}</strong></article>)}</div></> : <EmptyState title="No costs recorded" detail="Start with a subscription or registration fee you pay every month." action={<button className="primary-button" onClick={() => setCreating(true)}><Plus size={17} /> Record your first cost</button>} />}
    </Page>
    {creating && <Modal eyebrow="Money" title="Record a business cost" onClose={() => setCreating(false)}><form onSubmit={create}><div className="field-grid"><label>Description<input name="description" maxLength={200} required autoFocus /></label><label>Vendor<input name="vendor" maxLength={160} /></label><label>Amount<input name="amount" type="number" min="0" step="0.01" inputMode="decimal" required /></label><label>Date<input name="incurredOn" type="date" defaultValue={localDateValue()} required /></label><label>Category<input name="category" maxLength={100} placeholder="Software" /></label><label>Payment method<input name="paymentMethod" maxLength={100} placeholder="Business card" /></label><label>Renewal date<input name="renewalDate" type="date" /></label><label>Review state<select name="reviewState" defaultValue="ready"><option value="ready">Ready</option><option value="review">Needs review</option><option value="complete">Complete</option></select></label><label className="check-label"><input name="taxDeductible" type="checkbox" /> Tax-deductible</label><label className="check-label"><input name="recurring" type="checkbox" checked={recurring} onChange={(event) => setRecurring(event.target.checked)} /> Recurring cost</label>{recurring && <label>Repeats<select name="recurrence" defaultValue="monthly"><option value="monthly">Monthly</option><option value="quarterly">Quarterly</option><option value="yearly">Yearly</option></select></label>}</div><label>Notes<textarea name="notes" rows={3} maxLength={1000} /></label>{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? 'Saving...' : 'Save cost'}</button></div></form></Modal>}
  </>
}

export function localDateValue(date = new Date()) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
