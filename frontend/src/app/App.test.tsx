import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const landing = {
  buttons: [{ id: 'docs', label: 'Docs', description: 'Read the handbook.', href: '/docs', icon: 'globe' }],
  notifications: [],
}

function mockAPI(authenticated = true) {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.endsWith('/api/v1/me')) {
      return Promise.resolve(authenticated
        ? new Response(JSON.stringify({ email: 'joey@example.com', name: 'Joey Stout' }), { status: 200, headers: { 'Content-Type': 'application/json' } })
        : new Response(null, { status: 401 }))
    }
    if (path.endsWith('/api/v1/landing/buttons') && init?.method === 'POST') {
      return Promise.resolve(new Response(JSON.stringify({ id: 'reports', label: 'Fishing reports', description: 'Open the latest reports.', href: 'https://example.com/reports', icon: 'globe' }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    }
    return Promise.resolve(new Response(JSON.stringify(landing), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
}

describe('Kosmos landing zone', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  beforeEach(() => mockAPI())

  it('renders private module shortcuts for the signed-in user', async () => {
    render(<App />)

    await waitFor(() => expect(screen.getByText('Docs')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /sign out/i })).toBeInTheDocument()
    expect(screen.getByText('Read the handbook.')).toBeInTheDocument()
  })

  it('offers Google login without loading private workspace data', async () => {
    mockAPI(false)
    render(<App />)

    await waitFor(() => expect(screen.getByRole('link', { name: /sign in with google/i })).toBeInTheDocument())
    expect(screen.getByRole('link', { name: /sign in to add shortcuts/i })).toHaveAttribute('href', '/auth/login')
    expect(screen.queryByText('Docs')).not.toBeInTheDocument()
  })

  it.each([
    ['desktop', 1440],
    ['mobile', 390],
  ])('creates a shortcut and keeps navigation reachable on %s', async (_name, width) => {
    window.innerWidth = width
    render(<App />)

    await waitFor(() => expect(screen.getByText('Docs')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: /overview/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /contacts/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /documents/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add a shortcut/i }))
    expect(screen.getByRole('dialog', { name: /add a shortcut/i })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/button name/i), { target: { value: 'Fishing reports' } })
    fireEvent.change(screen.getByLabelText(/^link$/i), { target: { value: 'https://example.com/reports' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Open the latest reports.' } })
    fireEvent.click(screen.getByRole('button', { name: /save shortcut/i }))

    await waitFor(() => expect(screen.getByRole('link', { name: /fishing reports/i })).toHaveAttribute('href', 'https://example.com/reports'))
    expect(fetch).toHaveBeenCalledWith('/api/v1/landing/buttons', expect.objectContaining({ method: 'POST' }))
  })
})
