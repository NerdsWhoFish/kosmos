import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Documents } from './Documents'
import { Activity } from './Activity'
import { SearchResults } from './SearchResults'
import { Costs } from './Costs'
import { Shell } from '../components/Shell'

vi.mock('@uiw/react-codemirror', () => ({ default: () => null }))

const route = { id: '', action: 'list' }
const navigate = vi.fn()
const emptyResponses: Record<string, unknown> = {
  '/api/v1/documents': { documents: [] },
  '/api/v1/accounts': { accounts: [] },
  '/api/v1/contacts': { contacts: [] },
  '/api/v1/opportunities': { opportunities: [] },
  '/api/v1/costs': { costs: [] },
  '/api/v1/attachments': { attachments: [] },
  '/api/v1/activities': { activities: [] },
  '/api/v1/reminders': { reminders: [] },
  '/api/v1/search?q=first': { results: [] },
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks() })

describe('recoverable page loads', () => {
  it.each([
    { name: 'Documents', endpoint: '/api/v1/documents', page: <Documents route={route} accountID="" navigate={navigate} /> },
    { name: 'Activity and follow-ups', endpoint: '/api/v1/activities', page: <Activity futureOnly={false} navigate={navigate} /> },
    { name: 'Results for “first”', endpoint: '/api/v1/search?q=first', page: <SearchResults query="first" navigate={navigate} /> },
    { name: 'Business costs', endpoint: '/api/v1/costs', page: <Costs route={route} navigate={navigate} /> },
  ])('recovers $name after a failed load and retry', async ({ name, endpoint, page }) => {
    let fail = true
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path === endpoint && fail) {
        fail = false
        return Response.json({ error: { message: 'Temporary outage' } }, { status: 503 })
      }
      return Response.json(emptyResponses[path] ?? {})
    })
    vi.stubGlobal('fetch', fetcher)
    render(page)
    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { name })).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(fetcher.mock.calls.filter(([input]) => String(input) === endpoint)).toHaveLength(2)
  })

  it('keeps the current query result when an older request fails late', async () => {
    let rejectFirst!: (reason: Error) => void
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('q=first')) return new Promise<Response>((_, reject) => { rejectFirst = reject })
      return Promise.resolve(Response.json({ results: [{ id: 'current', kind: 'contact', title: 'Current result', href: '/contacts/current' }] }))
    }))
    const view = render(<SearchResults query="first" navigate={navigate} />)
    view.rerender(<SearchResults query="second" navigate={navigate} />)
    expect(await screen.findByText('Current result')).toBeInTheDocument()
    await act(async () => rejectFirst(new Error('Old query failed')))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('Current result')).toBeInTheDocument()
  })

  it('does not replace newer search results with an older success', async () => {
    let resolveFirst!: (response: Response) => void
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('q=first')) return new Promise<Response>((resolve) => { resolveFirst = resolve })
      return Promise.resolve(Response.json({ results: [{ id: 'current', kind: 'contact', title: 'Current result', href: '/contacts/current' }] }))
    }))
    const view = render(<SearchResults query="first" navigate={navigate} />)
    view.rerender(<SearchResults query="second" navigate={navigate} />)
    await screen.findByText('Current result')
    await act(async () => resolveFirst(Response.json({ results: [{ id: 'stale', kind: 'contact', title: 'Stale result', href: '/contacts/stale' }] })))
    expect(screen.queryByText('Stale result')).not.toBeInTheDocument()
    expect(screen.getByText('Current result')).toBeInTheDocument()
  })
})

it('names sign-out independently of hidden text and invokes logout', () => {
  const logout = vi.fn()
  render(<Shell user={{ name: 'Review Operator', email: 'review@example.com' }} modules={[]} path="/" navigate={navigate} logout={logout}>Workspace</Shell>)
  const button = screen.getByRole('button', { name: 'Sign out' })
  button.querySelector('span')!.style.display = 'none'
  expect(button).toHaveAccessibleName('Sign out')
  expect(button.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
  fireEvent.click(button)
  expect(logout).toHaveBeenCalledOnce()
})

it('preserves an unsaved document and requires review before retrying a conflict', async () => {
  const original = { id: 'document-1', title: 'Original title', body: 'Original body', revision: 1, links: [] }
  const latest = { ...original, title: 'Colleague title', body: 'Colleague changes', revision: 2 }
  let saves = 0
  let reads = 0
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/v1/documents' && !init?.method) return Response.json({ documents: [reads++ ? latest : original] })
    if (init?.method === 'PATCH') {
      saves++
      const draft = JSON.parse(String(init.body))
      expect(draft.expectedRevision).toBe(saves === 1 ? 1 : 2)
      expect(draft.body).toBe('My unsaved changes')
      if (saves === 1) return Response.json({ error: { code: 'document_conflict', message: 'A newer version was saved.' } }, { status: 409 })
      return Response.json({ ...latest, ...draft, revision: 3 })
    }
    return Response.json(emptyResponses[path] ?? {})
  }))
  render(<Documents route={{ id: 'document-1', action: 'edit' }} accountID="" navigate={navigate} />)
  const editor = await screen.findByLabelText('Start writing in Markdown')
  await waitFor(() => expect(screen.getByLabelText('Title')).toHaveValue('Original title'))
  fireEvent.change(editor, { target: { value: 'My unsaved changes' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save document' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Review latest version' }))
  expect(await screen.findByText('Colleague changes')).toBeInTheDocument()
  expect(editor).toHaveValue('My unsaved changes')
  expect(screen.getByRole('button', { name: 'Save document' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Keep my edits' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save document' }))
  await waitFor(() => expect(navigate).toHaveBeenCalledWith('/documents/document-1'))
  expect(saves).toBe(2)
})
