export type User = { email: string; name: string; picture?: string };
export type Shortcut = {
  id: string;
  label: string;
  description: string;
  href: string;
  icon: string;
};
export type Notification = {
  id: string;
  title: string;
  summary: string;
  kind: string;
  createdAt: string;
  href: string;
  readAt?: string;
};
export type Landing = { buttons: Shortcut[]; notifications: Notification[] };
export type Website = {
  url: string;
  domain: string;
  provider?: string;
  externalId?: string;
  renewalDate?: string;
  autoRenew: boolean;
  status?: string;
};
export type AccountLink = {
  label: string;
  url: string;
};
export type Account = {
  id: string;
  name: string;
  website?: string;
  websites: Website[];
  links: AccountLink[];
  billingEmail: string;
  status: "prospect" | "customer" | "inactive";
  notes: string;
  createdAt: string;
  updatedAt: string;
};
export type Contact = {
  id: string;
  accountId: string;
  name: string;
  email: string;
  phone: string;
  linkedinUrl: string;
  source: string;
  createdAt: string;
  updatedAt: string;
};
export type ContactSource = {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};
export type Opportunity = {
  id: string;
  name: string;
  accountId: string;
  contactId: string;
  amountCents: number;
  stage: string;
  nextStep: string;
  closeDate: string;
  ownerEmail: string;
  createdAt: string;
  updatedAt: string;
};
export type Activity = {
  id: string;
  contactId: string;
  opportunityId: string;
  kind: "note" | "call" | "email" | "meeting";
  body: string;
  occurredAt: string;
  createdAt: string;
};
export type Reminder = {
  id: string;
  accountId?: string;
  contactId: string;
  sourceKey?: string;
  title: string;
  dueAt: string;
  completed: boolean;
  ownerEmail: string;
  createdAt: string;
  updatedAt: string;
};
export type RecordLink = {
  type: "account" | "contact" | "opportunity" | "cost" | "document";
  id: string;
};
export type Document = {
  id: string;
  title: string;
  body: string;
  links: RecordLink[];
  revision: number;
  createdAt: string;
  updatedAt: string;
};
export type DocumentRevision = {
  id: string;
  documentId: string;
  title: string;
  body: string;
  links: RecordLink[];
  revision: number;
  createdAt: string;
};
export type Cost = {
  id: string;
  vendor: string;
  description: string;
  amountCents: number;
  category: string;
  incurredOn: string;
  recurring: boolean;
  recurrence: string;
  taxDeductible: boolean;
  notes: string;
  renewalDate: string;
  paymentMethod: string;
  reviewState: "ready" | "review" | "complete";
  createdAt: string;
  updatedAt: string;
};
export type Summary = {
  contacts: number;
  openOpportunities: number;
  pipelineAmountCents: number;
  wonOpportunities: number;
  wonAmountCents: number;
  lostOpportunities: number;
  lostAmountCents: number;
  followUpsDue: number;
  currentMonthCostCents: number;
  recentActivities: Activity[];
};
export type SearchResult = {
  id: string;
  kind: string;
  title: string;
  subtitle: string;
  href: string;
};
export type Member = {
  id: string;
  email: string;
  name: string;
  role: "owner" | "admin" | "member" | "viewer";
  status: "active" | "disabled";
  createdAt: string;
  updatedAt: string;
};
export type PipelineStage = {
  id: string;
  name: string;
  position: number;
  probability: number;
  closed: boolean;
  won: boolean;
  createdAt: string;
  updatedAt: string;
};
export type EmailTemplate = {
  id: string;
  name: string;
  subject: string;
  body: string;
  createdAt: string;
  updatedAt: string;
};
export type GoogleConnection = {
  id: string;
  userEmail: string;
  googleEmail: string;
  tiller?: { spreadsheetId: string; range: string };
  lastMailSyncAt?: string;
  createdAt: string;
  updatedAt: string;
};
export type GoogleStatus = {
  connected: boolean;
  connection: GoogleConnection | null;
  connectUrl: string;
};
export type GoogleContactsStatus = {
  connected: boolean;
  googleEmail: string;
  connectUrl: string;
  pending: number;
  failed: number;
  synced: number;
};
export type SendAsMapping = {
  id: string;
  memberId: string;
  memberEmail: string;
  email: string;
  updatedBy: string;
  createdAt: string;
  updatedAt: string;
};
export type CloudflareStatus = {
  connected: boolean;
  accountId?: string;
  domainCount?: number;
};
export type CloudflareDomain = {
  domainName: string;
  zoneId?: string;
  registered: boolean;
  renewalDate?: string;
  autoRenew: boolean;
  status?: string;
};
export type TillerWebhookStatus = { connected: boolean; endpoint: string };
export type TillerProductMapping = {
  id: string;
  productId: string;
  productName?: string;
  accountId: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
};
export type MailMessage = {
  id: string;
  from: string;
  subject: string;
  snippet: string;
  receivedAt: string;
  contactId?: string;
  threadId: string;
  createdAt: string;
};
export type Transaction = {
  id: string;
  externalId: string;
  date: string;
  description: string;
  merchant: string;
  amountCents: number;
  source: string;
  matchStatus: "matched" | "review" | "ignored";
  accountId?: string;
  contactId?: string;
  costId?: string;
  createdAt: string;
  updatedAt: string;
};
export type Attachment = {
  id: string;
  fileName: string;
  contentType: string;
  size: number;
  kind: string;
  recordType: string;
  recordId: string;
  createdBy: string;
  createdAt: string;
  downloadUrl: string;
  viewUrl: string;
};
export type AuditEntry = {
  id: string;
  actor: string;
  action: string;
  entityType: string;
  entityId: string;
  summary: string;
  createdAt: string;
};
export type ModuleManifest = {
  name: string;
  navigation: { path: string; label: string; icon: string }[];
  permissions: string[];
  resources: string[];
  eventTypes?: string[];
  backgroundJobs?: string[];
  searchProviders?: string[];
  documentLinkTargets?: string[];
};
export type AcceptedJob = { id: string; status: "accepted" };

type APIError = { error?: string | { message?: string } };
type PageMetadata = { nextCursor?: string };
type PaginatedBody = Record<string, unknown> & { page: PageMetadata };

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? "GET").toUpperCase();
  const headers = new Headers(init.headers);
  if (
    init.body &&
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type")
  )
    headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD"].includes(method)) headers.set("X-Kosmos-CSRF", "1");
  if (method === "POST" && !headers.has("Idempotency-Key"))
    headers.set("Idempotency-Key", requestID());
  const request = { ...init, headers };
  const firstPage = await fetchJSON<T>(path, request);
  if (method !== "GET") return firstPage;
  return collectPages(path, request, firstPage);
}

async function fetchJSON<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;
    try {
      const body = (await response.json()) as APIError;
      message =
        typeof body.error === "string"
          ? body.error
          : (body.error?.message ?? message);
    } catch {
      message = response.status === 401 ? "Please sign in again." : message;
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

async function collectPages<T>(
  path: string,
  init: RequestInit,
  firstPage: T,
): Promise<T> {
  const collection = paginatedCollection(firstPage);
  if (!collection) return firstPage;

  const [key, items, page] = collection;
  let cursor = page.nextCursor;
  const seen = new Set<string>();
  while (cursor) {
    if (seen.has(cursor))
      throw new Error("Paginated response repeated a cursor");
    seen.add(cursor);
    const nextPage = await fetchJSON<unknown>(withCursor(path, cursor), init);
    const nextCollection = paginatedCollection(nextPage);
    if (!nextCollection || nextCollection[0] !== key)
      throw new Error("Paginated response changed collection");
    items.push(...nextCollection[1]);
    cursor = nextCollection[2].nextCursor;
    (firstPage as PaginatedBody).page = nextCollection[2];
  }
  return firstPage;
}

function paginatedCollection(
  value: unknown,
): [string, unknown[], PageMetadata] | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return undefined;
  const body = value as Record<string, unknown>;
  if (!body.page || typeof body.page !== "object" || Array.isArray(body.page))
    return undefined;
  const arrays = Object.entries(body).filter(
    ([key, item]) => key !== "page" && Array.isArray(item),
  );
  if (arrays.length !== 1) return undefined;
  return [arrays[0][0], arrays[0][1] as unknown[], body.page as PageMetadata];
}

function withCursor(path: string, cursor: string) {
  const url = new URL(path, globalThis.location?.origin ?? "http://localhost");
  url.searchParams.set("cursor", cursor);
  if (/^https?:\/\//i.test(path)) return url.toString();
  return `${url.pathname}${url.search}${url.hash}`;
}

function requestID() {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;
}

export function money(cents: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 2,
  }).format(cents / 100);
}

export function shortDate(value: string) {
  if (!value) return "";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(value));
}
