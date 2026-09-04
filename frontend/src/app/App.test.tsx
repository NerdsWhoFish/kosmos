import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const user = { email: 'joey@nerdswhofish.com', name: 'Joey Stout' }
const contact = { id: 'contact-1', name: 'Ada Angler', company: 'River Labs', email: 'ada@example.com', phone: '', status: 'lead', createdAt: '2026-09-03T12:00:00Z', updatedAt: '2026-09-03T12:00:00Z' }
const responses: Record<string, unknown> = {
  '/api/v1/summary': { contacts: 1, openOpportunities: 1, pipelineAmountCents: 125000, followUpsDue: 1, currentMonthCostCents: 1800, recentActivities: [] },
  '/api/v1/landing': { buttons: [{ id: 'docs', label: 'Field notes', description: 'Open the handbook.', href: '/documents', icon: 'globe' }], notifications: [] },
  '/api/v1/contacts': { contacts: [contact] },
  '/api/v1/opportunities': { opportunities: [{ id: 'opportunity-1', name: 'Website refresh', contactId: contact.id, amountCents: 125000, stage: 'qualified', nextStep: 'Send proposal', closeDate: '', createdAt: '2026-09-03T12:00:00Z', updatedAt: '2026-09-03T12:00:00Z' }] },
  '/api/v1/activities': { activities: [] },
  '/api/v1/reminders': { reminders: [{ id: 'reminder-1', contactId: contact.id, title: 'Send proposal', dueAt: '2026-09-03T12:00:00Z', completed: false, createdAt: '2026-09-03T12:00:00Z', updatedAt: '2026-09-03T12:00:00Z' }] },
  '/api/v1/documents': { documents: [{ id: 'document-1', title: 'Client kickoff', body: '# Agenda', createdAt: '2026-09-03T12:00:00Z', updatedAt: '2026-09-03T12:00:00Z' }] },
  '/api/v1/costs': { costs: [{ id: 'cost-1', vendor: 'Google', description: 'Workspace', amountCents: 1800, category: 'Software', incurredOn: '2026-09-03', recurring: true, recurrence: 'monthly', taxDeductible: true, notes: '', createdAt: '2026-09-03T12:00:00Z', updatedAt: '2026-09-03T12:00:00Z' }] },
  '/api/v1/search?q=river': { results: [{ id: contact.id, kind: 'contact', title: contact.name, subtitle: contact.company, href: '/contacts' }] },
}

function mockAPI(authenticated = true) {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://kosmos.test')
    const method = (init?.method ?? 'GET').toUpperCase()
    const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : {}
    if (url.pathname === '/api/v1/me') return Promise.resolve(authenticated ? json(user) : new Response(null, { status: 401 }))
    if (url.pathname === '/auth/logout') return Promise.resolve(new Response(null, { status: 204 }))
    if (url.pathname === '/api/v1/contacts' && method === 'POST') return Promise.resolve(json({ ...contact, id: 'contact-2', name: 'Grace Hopper', company: 'Compiler Co' }, 201))
    if (url.pathname === '/api/v1/contacts/contact-2' && method === 'PATCH') return Promise.resolve(json({ ...contact, id: 'contact-2', name: 'Grace Hopper', company: 'Compiler Co', status: body.status }))
    if (url.pathname === '/api/v1/activities' && method === 'POST') return Promise.resolve(json({ id: 'activity-2', contactId: body.contactId, opportunityId: '', kind: body.kind, body: body.body, occurredAt: '2026-09-03T13:00:00Z', createdAt: '2026-09-03T13:00:00Z' }, 201))
    if (url.pathname === '/api/v1/reminders' && method === 'POST') return Promise.resolve(json({ id: 'reminder-2', contactId: body.contactId, title: body.title, dueAt: body.dueAt, completed: false, createdAt: '2026-09-03T13:00:00Z', updatedAt: '2026-09-03T13:00:00Z' }, 201))
    if (url.pathname === '/api/v1/reminders/reminder-1' && method === 'PATCH') return Promise.resolve(json({ ...(responses['/api/v1/reminders'] as { reminders: unknown[] }).reminders[0] as object, completed: true }))
    if (url.pathname === '/api/v1/opportunities' && method === 'POST') return Promise.resolve(json({ id: 'opportunity-2', name: body.name, contactId: body.contactId, amountCents: body.amountCents, stage: body.stage, nextStep: body.nextStep, closeDate: body.closeDate, createdAt: '2026-09-03T13:00:00Z', updatedAt: '2026-09-03T13:00:00Z' }, 201))
    if (url.pathname === '/api/v1/documents' && method === 'POST') return Promise.resolve(json({ id: 'document-2', title: body.title, body: body.body, createdAt: '2026-09-03T13:00:00Z', updatedAt: '2026-09-03T13:00:00Z' }, 201))
    if (url.pathname === '/api/v1/costs' && method === 'POST') return Promise.resolve(json({ id: 'cost-2', vendor: body.vendor, description: body.description, amountCents: body.amountCents, category: body.category, incurredOn: body.incurredOn, recurring: body.recurring, recurrence: body.recurrence, taxDeductible: body.taxDeductible, notes: body.notes, createdAt: '2026-09-03T13:00:00Z', updatedAt: '2026-09-03T13:00:00Z' }, 201))
    if (url.pathname === '/api/v1/landing/buttons' && method === 'POST') return Promise.resolve(json({ id: 'reports', label: 'Fishing reports', description: 'Open reports.', href: 'https://example.com/reports', icon: 'globe' }, 201))
    const key = url.pathname + url.search
    return Promise.resolve(json(responses[key] ?? responses[url.pathname] ?? {}))
  }))
}

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('Kosmos application', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
    vi.stubGlobal('scrollTo', vi.fn())
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('shows no private workspace information before login', async () => {
    mockAPI(false)
    render(<App />)

    expect(await screen.findByRole('link', { name: /continue with google/i })).toHaveAttribute('href', '/auth/login')
    expect(screen.getByText(/approved company google accounts/i)).toBeInTheDocument()
    expect(screen.queryByText('$1,250.00')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: /workspace/i })).not.toBeInTheDocument()
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['desktop', 1440],
    ['mobile', 390],
  ])('keeps every core workspace workflow usable on %s', async (_name, width) => {
    window.innerWidth = width
    mockAPI()
    render(<App />)

    expect(await screen.findByRole('heading', { name: /good (morning|afternoon|evening), joey/i })).toBeInTheDocument()
    for (const destination of ['Overview', 'Contacts', 'Opportunities', 'Documents', 'Costs', 'Settings']) {
      expect(screen.getByRole('link', { name: destination })).toBeInTheDocument()
    }

    fireEvent.click(screen.getByRole('link', { name: 'Contacts' }))
    expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /add contact/i }))
    fireEvent.change(screen.getByLabelText(/full name/i), { target: { value: 'Grace Hopper' } })
    fireEvent.change(screen.getByLabelText(/company/i), { target: { value: 'Compiler Co' } })
    fireEvent.click(screen.getByRole('button', { name: /save contact/i }))
    expect(await screen.findByRole('heading', { name: 'Grace Hopper' })).toBeInTheDocument()

    fireEvent.change(screen.getByRole('combobox', { name: /relationship status/i }), { target: { value: 'customer' } })
    await waitFor(() => expect(screen.getByRole('combobox', { name: /relationship status/i })).toHaveValue('customer'))
    fireEvent.change(screen.getByRole('textbox', { name: /activity note/i }), { target: { value: 'Confirmed the project scope.' } })
    fireEvent.click(screen.getByRole('button', { name: /add to timeline/i }))
    expect(await screen.findByText('Confirmed the project scope.')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: /what needs to happen/i }), { target: { value: 'Send the estimate' } })
    fireEvent.change(screen.getByLabelText(/when/i), { target: { value: '2026-09-10T10:00' } })
    fireEvent.click(screen.getByRole('button', { name: /add reminder/i }))
    expect(await screen.findByText('Send the estimate')).toBeInTheDocument()

    const contactRequest = vi.mocked(fetch).mock.calls.find(([input, init]) => String(input) === '/api/v1/contacts' && init?.method === 'POST')
    expect(new Headers(contactRequest?.[1]?.headers).get('X-Kosmos-CSRF')).toBe('1')

    fireEvent.click(screen.getByRole('link', { name: 'Opportunities' }))
    expect(await screen.findByRole('heading', { name: 'Opportunities' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /add opportunity/i }))
    fireEvent.change(screen.getByRole('textbox', { name: /opportunity name/i }), { target: { value: 'River cleanup' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: /value/i }), { target: { value: '2500' } })
    fireEvent.click(screen.getByRole('button', { name: /save opportunity/i }))
    expect(await screen.findByRole('heading', { name: 'River cleanup' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('link', { name: 'Documents' }))
    expect(await screen.findByRole('heading', { name: 'Documents' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /new document/i }))
    fireEvent.change(screen.getByRole('textbox', { name: /^title$/i }), { target: { value: 'Fishing checklist' } })
    fireEvent.change(screen.getByRole('textbox', { name: /start writing in markdown/i }), { target: { value: '# Before launch' } })
    fireEvent.click(screen.getByRole('button', { name: /create document/i }))
    expect(await screen.findByRole('heading', { name: 'Before launch' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('link', { name: 'Costs' }))
    expect(await screen.findByRole('heading', { name: 'Business costs' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /record a cost/i }))
    fireEvent.change(screen.getByRole('textbox', { name: /description/i }), { target: { value: 'Domain renewal' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: /amount/i }), { target: { value: '24' } })
    fireEvent.click(screen.getByRole('button', { name: /save cost/i }))
    expect(await screen.findByText('Domain renewal')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /notifications and follow-ups/i }))
    expect(await screen.findByRole('heading', { name: 'Activity and follow-ups' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /complete send proposal/i }))
    await waitFor(() => expect(screen.queryByRole('button', { name: /complete send proposal/i })).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('link', { name: 'Settings' }))
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })

  it('searches the private workspace and opens a result', async () => {
    mockAPI()
    render(<App />)
    await screen.findByRole('heading', { name: /good (morning|afternoon|evening)/i })
    const search = screen.getByRole('textbox', { name: /search kosmos/i })
    fireEvent.change(search, { target: { value: 'river' } })
    fireEvent.submit(search.closest('form')!)
    expect(await screen.findByRole('heading', { name: /results for “river”/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /ada angler/i })).toBeInTheDocument()
  })

  it('creates a landing-zone shortcut with CSRF protection', async () => {
    mockAPI()
    render(<App />)
    await screen.findByRole('heading', { name: /good (morning|afternoon|evening)/i })
    fireEvent.click(screen.getByRole('button', { name: /add a shortcut/i }))
    fireEvent.change(screen.getByLabelText(/button name/i), { target: { value: 'Fishing reports' } })
    fireEvent.change(screen.getByLabelText(/^link$/i), { target: { value: 'https://example.com/reports' } })
    fireEvent.click(screen.getByRole('button', { name: /save shortcut/i }))
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([input]) => String(input) === '/api/v1/landing/buttons')).toBe(true))
    const request = vi.mocked(fetch).mock.calls.find(([input]) => String(input) === '/api/v1/landing/buttons')
    expect(new Headers(request?.[1]?.headers).get('X-Kosmos-CSRF')).toBe('1')
  })
})
