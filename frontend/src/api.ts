export type User = { email: string; name: string; picture?: string }
export type Shortcut = { id: string; label: string; description: string; href: string; icon: string }
export type Notification = { id: string; title: string; summary: string; kind: string; createdAt: string; href: string }
export type Landing = { buttons: Shortcut[]; notifications: Notification[] }
export type Contact = { id: string; name: string; company: string; email: string; phone: string; status: 'lead' | 'prospect' | 'customer'; createdAt: string; updatedAt: string }
export type Opportunity = { id: string; name: string; contactId: string; amountCents: number; stage: 'new' | 'qualified' | 'proposal' | 'won' | 'lost'; nextStep: string; closeDate: string; createdAt: string; updatedAt: string }
export type Activity = { id: string; contactId: string; opportunityId: string; kind: 'note' | 'call' | 'email' | 'meeting'; body: string; occurredAt: string; createdAt: string }
export type Reminder = { id: string; contactId: string; title: string; dueAt: string; completed: boolean; createdAt: string; updatedAt: string }
export type Document = { id: string; title: string; body: string; createdAt: string; updatedAt: string }
export type Cost = { id: string; vendor: string; description: string; amountCents: number; category: string; incurredOn: string; recurring: boolean; recurrence: string; taxDeductible: boolean; notes: string; createdAt: string; updatedAt: string }
export type Summary = { contacts: number; openOpportunities: number; pipelineAmountCents: number; followUpsDue: number; currentMonthCostCents: number; recentActivities: Activity[] }
export type SearchResult = { id: string; kind: string; title: string; subtitle: string; href: string }

type APIError = { error?: string | { message?: string } }

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase()
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (!['GET', 'HEAD'].includes(method)) headers.set('X-Kosmos-CSRF', '1')
  const response = await fetch(path, { ...init, headers })
  if (!response.ok) {
    let message = `Request failed with status ${response.status}`
    try {
      const body = await response.json() as APIError
      message = typeof body.error === 'string' ? body.error : body.error?.message ?? message
    } catch {
      message = response.status === 401 ? 'Please sign in again.' : message
    }
    throw new Error(message)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function money(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(cents / 100)
}

export function shortDate(value: string) {
  if (!value) return ''
  return new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' }).format(new Date(value))
}
