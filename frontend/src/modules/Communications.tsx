import { FormEvent, useCallback, useEffect, useState } from 'react'
import { ExternalLink, Mail, MessageSquareText, Plus, RefreshCw, Send } from 'lucide-react'
import { api, EmailTemplate, GoogleStatus, MailMessage, Notification, shortDate } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function Communications() {
  const [status, setStatus] = useState<GoogleStatus | null>(null)
  const [templates, setTemplates] = useState<EmailTemplate[]>([])
  const [messages, setMessages] = useState<MailMessage[]>([])
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [templateOpen, setTemplateOpen] = useState(false)
  const [sending, setSending] = useState(false)
  const [phone, setPhone] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([
      api<GoogleStatus>('/api/v1/integrations/google'),
      api<{ templates: EmailTemplate[] }>('/api/v1/email/templates'),
      api<{ messages: MailMessage[] }>('/api/v1/email/messages'),
      api<{ notifications: Notification[] }>('/api/v1/notifications'),
    ]).then(([connection, templateResult, messageResult, notificationResult]) => {
      setStatus(connection)
      setTemplates(templateResult.templates)
      setMessages(messageResult.messages)
      setNotifications(notificationResult.notifications)
      setError('')
    }).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])

  useEffect(load, [load])

  async function sendEmail(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
	const formElement = event.currentTarget
    setSending(true)
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      await api('/api/v1/email/send', { method: 'POST', body: JSON.stringify({ to: form.get('to'), subject: form.get('subject'), body: form.get('body') }) })
      formElement.reset()
      setNotice('Email sent through your Google account.')
      load()
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : 'Could not send email')
    } finally {
      setSending(false)
    }
  }

  async function syncMail() {
    setNotice('Checking Gmail for customer replies...')
    try {
      const result = await api<{ newMessages: number }>('/api/v1/email/sync', { method: 'POST' })
      setNotice(result.newMessages ? `${result.newMessages} new customer email${result.newMessages === 1 ? '' : 's'} found.` : 'No new customer email found.')
      load()
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : 'Could not sync Gmail')
    }
  }

  async function createTemplate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    await api('/api/v1/email/templates', { method: 'POST', body: JSON.stringify({ name: form.get('name'), subject: form.get('subject'), body: form.get('body') }) })
    setTemplateOpen(false)
    load()
  }

  async function markRead(item: Notification) {
    await api(`/api/v1/notifications/${item.id}`, { method: 'PATCH', body: '{}' })
    setNotifications((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, readAt: new Date().toISOString() } : candidate))
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} retry={load} />

  return <Page eyebrow="Conversations" title="Communications" detail="Send intentional emails, notice customer replies, and jump into Google Voice without turning Kosmos into another inbox.">
    {!status?.connected && <section className="tip-banner integration-banner"><span className="tip-icon"><Mail size={20} /></span><span><strong>Connect Google Workspace</strong><small>Grant Gmail compose and metadata access plus read-only Tiller sheet access. Kosmos never stores message bodies from your inbox.</small></span><a className="banner-button" href={status?.connectUrl ?? '/auth/connect/workspace'}>Connect Google <ExternalLink size={15} /></a></section>}
    {status?.connected && <div className="status-strip"><span className="security-dot" /><strong>{status.connection?.googleEmail}</strong> is connected <button className="text-button" onClick={syncMail}><RefreshCw size={15} /> Check for replies</button></div>}
    {notice && <p className="inline-notice" role="status">{notice}</p>}
    <section className="split-grid">
      <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Outbound</p><h2>Send one good email</h2></div><button className="text-button" onClick={() => setTemplateOpen(true)}><Plus size={15} /> Template</button></div>
        <form onSubmit={sendEmail}><label>To<input name="to" type="email" required placeholder="customer@example.com" /></label><label>Subject<input name="subject" required maxLength={200} /></label><label>Message<textarea name="body" required rows={9} /></label><div className="form-actions"><button className="primary-button" disabled={!status?.connected || sending}><Send size={16} /> {sending ? 'Sending...' : 'Send with Gmail'}</button></div></form>
        {!!templates.length && <div className="template-list"><p className="eyebrow">Saved templates</p>{templates.map((template) => <button key={template.id} className="record-row compact" type="button" onClick={() => setNotice(`Use “${template.name}” by copying its subject and message into the email above.`)}><strong>{template.name}</strong><small>{template.subject}</small></button>)}</div>}
      </div>
      <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Inbound</p><h2>Customer replies</h2></div></div>{messages.length ? <div className="activity-list">{messages.map((message) => <article className="activity-item" key={message.id}><span className="activity-icon"><Mail size={16} /></span><span><strong>{message.subject || '(No subject)'}</strong><small>{message.from}</small><time>{shortDate(message.receivedAt)}</time></span></article>)}</div> : <EmptyState title="No customer replies yet" detail="When a known contact emails your connected Gmail account, the metadata appears here." />}</div>
    </section>
    <section className="split-grid lower-grid">
      <div className="panel"><p className="eyebrow">Google Voice</p><h2>Call or text a contact</h2><p className="muted-copy">Kosmos opens the native phone or messaging app, or takes you to Google Voice. It does not pretend Google Voice has an automation API.</p><label>Phone number<input value={phone} onChange={(event) => setPhone(event.target.value)} inputMode="tel" placeholder="+1 555 123 4567" /></label><div className="button-row"><a className={`secondary-button ${phone ? '' : 'disabled'}`} href={phone ? `tel:${phone}` : undefined}>Call</a><a className={`secondary-button ${phone ? '' : 'disabled'}`} href={phone ? `sms:${phone}` : undefined}>Text</a><a className="primary-button" href="https://voice.google.com/u/0/messages" target="_blank" rel="noreferrer">Open Google Voice <ExternalLink size={15} /></a></div></div>
      <div className="panel"><p className="eyebrow">Across Kosmos</p><h2>Notifications</h2>{notifications.length ? <div className="activity-list">{notifications.slice(0, 8).map((item) => <article className={`activity-item ${item.readAt ? 'is-read' : ''}`} key={item.id}><span className="activity-icon lavender"><MessageSquareText size={16} /></span><span><strong>{item.title}</strong><small>{item.summary}</small><time>{shortDate(item.createdAt)}</time></span>{!item.readAt && <button className="text-button" onClick={() => markRead(item)}>Mark read</button>}</article>)}</div> : <EmptyState title="You are all caught up" detail="Leads, email, transactions, and reminders will collect here." />}</div>
    </section>
    {templateOpen && <Modal eyebrow="Reusable message" title="New email template" onClose={() => setTemplateOpen(false)}><form onSubmit={createTemplate}><label>Name<input name="name" required autoFocus /></label><label>Subject<input name="subject" required /></label><label>Message<textarea name="body" rows={8} required /></label><div className="form-actions"><button type="button" className="secondary-button" onClick={() => setTemplateOpen(false)}>Cancel</button><button className="primary-button">Save template</button></div></form></Modal>}
  </Page>
}
