import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

describe('Kosmos landing zone', () => {
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
})
