import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

describe('Kosmos landing zone', () => {
  afterEach(cleanup)

  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith('/api/v1/me')) return Promise.resolve(new Response(null, { status: 401 }))
      return Promise.resolve(new Response(JSON.stringify({ buttons: [{ id: 'docs', label: 'Docs', description: 'Read the handbook.', href: '/docs', icon: 'globe' }], notifications: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    }))
  })

  it('renders module-provided shortcuts and Google sign-in', async () => {
    render(<App />)

    await waitFor(() => expect(screen.getByText('Docs')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: /sign in with google/i })).toHaveAttribute('href', '/auth/login')
    expect(screen.getByText('Read the handbook.')).toBeInTheDocument()
  })

  it.each([
    ['desktop', 1440],
    ['mobile', 390],
  ])('keeps primary navigation reachable on %s', async (_name, width) => {
    window.innerWidth = width
    render(<App />)

    await waitFor(() => expect(screen.getByText('Docs')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: /overview/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /contacts/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /documents/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /sign in with google/i })).toBeInTheDocument()
  })
})
