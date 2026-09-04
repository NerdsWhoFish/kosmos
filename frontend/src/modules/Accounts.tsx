import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  Building2,
  Clock3,
  Cloud,
  ExternalLink,
  FilePlus2,
  FileText,
  Globe2,
  Link2,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import {
  Account,
  AccountEvent,
  AccountLink,
  api,
  apiPage,
  CloudflareDomain,
  CloudflareStatus,
  Contact,
  Document,
  money,
  Opportunity,
  PageMetadata,
  Website,
} from "../api";
import { ContactSourcePicker } from "../components/ContactSourcePicker";
import { RecordPhoto } from "../components/RecordPhoto";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { WorkflowPage } from "../components/WorkflowPage";
import { ResourceRoute } from "../routing";

type AccountDetail = {
  account: Account;
  contacts: Contact[];
  opportunities: Opportunity[];
  documents: Document[];
};
type AccountCreation = { account: Account; contact?: Contact };

export function Accounts({
  route,
  navigate,
}: {
  route: ResourceRoute;
  navigate: (path: string) => void;
}) {
  const [items, setItems] = useState<Account[]>([]);
  const [selected, setSelected] = useState<AccountDetail | null>(null);
  const [recentEvents, setRecentEvents] = useState<AccountEvent[]>([]);
  const [cloudflare, setCloudflare] = useState<CloudflareStatus | null>(null);
  const [domains, setDomains] = useState<CloudflareDomain[]>([]);
  const [websiteFields, setWebsiteFields] = useState([""]);
  const [editWebsiteFields, setEditWebsiteFields] = useState([""]);
  const [editLinkFields, setEditLinkFields] = useState<AccountLink[]>([
    { label: "", url: "" },
  ]);
  const [includeContact, setIncludeContact] = useState(true);
  const [selectedDomain, setSelectedDomain] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingDomains, setLoadingDomains] = useState(false);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const load = useCallback(
    () =>
      Promise.all([
        api<{ accounts: Account[] }>("/api/v1/accounts"),
        api<CloudflareStatus>("/api/v1/integrations/cloudflare"),
      ])
        .then(([response, connection]) => {
          setItems(response.accounts);
          setCloudflare(connection);
          setError("");
        })
        .catch((reason: Error) => setError(reason.message))
        .finally(() => setLoading(false)),
    [],
  );

  const loadSelected = useCallback(
    (id: string) =>
      api<AccountDetail>(`/api/v1/accounts/${id}`)
        .then((detail) => {
          setSelected(detail);
        })
        .catch((reason: Error) => setError(reason.message)),
    [],
  );

  const loadRecentEvents = useCallback((id: string) => {
    void apiPage<{ events: AccountEvent[] }>(
      `/api/v1/accounts/${id}/events?limit=5`,
    )
      .then((timeline) => setRecentEvents(timeline.events ?? []))
      .catch(() => setRecentEvents([]));
  }, []);

  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (!route.id) {
      setSelected(null);
      setRecentEvents([]);
      return;
    }
    void loadSelected(route.id);
    loadRecentEvents(route.id);
  }, [route.id, loadRecentEvents, loadSelected]);
  useEffect(() => {
    if (!selected) return;
    if (route.action === "edit") {
      const websites = accountWebsites(selected.account);
      setEditWebsiteFields(
        websites.length ? websites.map((item) => item.url) : [""],
      );
    }
    if (route.action === "links") {
      setEditLinkFields(
        selected.account.links?.length
          ? selected.account.links.map((link) => ({ ...link }))
          : [{ label: "", url: "" }],
      );
    }
    if (route.action === "domain") void loadDomains();
  }, [route.action, selected?.account.id]);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    const primaryContact = includeContact
      ? {
          name: form.get("contactName"),
          email: form.get("contactEmail"),
          phone: form.get("contactPhone"),
          linkedinUrl: form.get("contactLinkedInUrl"),
          source: form.get("contactSource"),
        }
      : undefined;
    try {
      const created = await api<AccountCreation>("/api/v1/accounts", {
        method: "POST",
        body: JSON.stringify({
          name: form.get("name"),
          websites: websiteFields
            .filter((url) => url.trim())
            .map((url) => ({ url })),
          billingEmail: form.get("billingEmail"),
          status: form.get("status"),
          notes: form.get("notes"),
          primaryContact,
        }),
      });
      setItems((current) => [created.account, ...current]);
      resetCreateForm();
      navigate(`/accounts/${created.account.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save account",
      );
    } finally {
      setSaving(false);
    }
  }

  async function loadDomains() {
    setLoadingDomains(true);
    setFormError("");
    try {
      const response = await api<{ domains: CloudflareDomain[] }>(
        "/api/v1/integrations/cloudflare/domains",
      );
      setDomains(response.domains);
      setSelectedDomain(response.domains[0]?.domainName ?? "");
    } catch (reason) {
      setFormError(
        reason instanceof Error
          ? reason.message
          : "Could not load Cloudflare domains",
      );
    } finally {
      setLoadingDomains(false);
    }
  }

  async function linkDomain(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    try {
      const response = await api<{ account: Account }>(
        "/api/v1/integrations/cloudflare/link",
        {
          method: "POST",
          body: JSON.stringify({
            accountId: selected.account.id,
            domainName: form.get("domainName"),
            renewalDate: form.get("renewalDate"),
          }),
        },
      );
      setSelected((current) =>
        current ? { ...current, account: response.account } : current,
      );
      setItems((current) =>
        current.map((item) =>
          item.id === response.account.id ? response.account : item,
        ),
      );
      loadRecentEvents(response.account.id);
      navigate(`/accounts/${response.account.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not link domain",
      );
    } finally {
      setSaving(false);
    }
  }

  async function updateStatus(status: Account["status"]) {
    if (!selected) return;
    setSaving(true);
    setFormError("");
    try {
      const account = await api<Account>(
        `/api/v1/accounts/${selected.account.id}`,
        { method: "PATCH", body: JSON.stringify({ status }) },
      );
      setSelected((current) => (current ? { ...current, account } : current));
      setItems((current) =>
        current.map((item) => (item.id === account.id ? account : item)),
      );
      loadRecentEvents(account.id);
    } catch (reason) {
      setFormError(
        reason instanceof Error
          ? reason.message
          : "Could not update the relationship",
      );
    } finally {
      setSaving(false);
    }
  }

  async function updateLinks(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError("");
    try {
      const account = await api<Account>(
        `/api/v1/accounts/${selected.account.id}`,
        {
          method: "PATCH",
          body: JSON.stringify({
            links: editLinkFields.filter(
              (link) => link.label.trim() || link.url.trim(),
            ),
          }),
        },
      );
      setSelected((current) => (current ? { ...current, account } : current));
      setItems((current) =>
        current.map((item) => (item.id === account.id ? account : item)),
      );
      loadRecentEvents(account.id);
      navigate(`/accounts/${account.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not update links",
      );
    } finally {
      setSaving(false);
    }
  }

  async function updateAccount(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    try {
      const account = await api<Account>(
        `/api/v1/accounts/${selected.account.id}`,
        {
          method: "PATCH",
          body: JSON.stringify({
            name: form.get("name"),
            billingEmail: form.get("billingEmail"),
            status: form.get("status"),
            notes: form.get("notes"),
            websites: editWebsiteFields
              .filter((url) => url.trim())
              .map((url) => ({ url })),
          }),
        },
      );
      setSelected((current) => (current ? { ...current, account } : current));
      setItems((current) =>
        current.map((item) => (item.id === account.id ? account : item)),
      );
      loadRecentEvents(account.id);
      navigate(`/accounts/${account.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not update account",
      );
    } finally {
      setSaving(false);
    }
  }

  async function deleteAccount() {
    if (!selected) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/accounts/${selected.account.id}`, {
        method: "DELETE",
      });
      setItems((current) =>
        current.filter((item) => item.id !== selected.account.id),
      );
      setSelected(null);
      navigate("/accounts");
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not delete account",
      );
    } finally {
      setSaving(false);
    }
  }

  function resetCreateForm() {
    setWebsiteFields([""]);
    setIncludeContact(true);
    setFormError("");
  }

  if (loading) return <LoadingState />;
  if (error) return <ErrorState message={error} retry={load} />;
  if (selected)
    return (
      <>
        {route.action === "view" && (
          <AccountView
            detail={selected}
            events={recentEvents}
            cloudflare={cloudflare}
            navigate={navigate}
            saving={saving}
            formError={formError}
            onStatus={updateStatus}
            onLink={() => navigate(`/accounts/${selected.account.id}/domain`)}
            onEdit={() => navigate(`/accounts/${selected.account.id}/edit`)}
            onManageLinks={() =>
              navigate(`/accounts/${selected.account.id}/links`)
            }
            onDelete={() => navigate(`/accounts/${selected.account.id}/delete`)}
          />
        )}
        {route.action === "events" && (
          <AccountEventsPage account={selected.account} navigate={navigate} />
        )}
        {route.action === "edit" && (
          <WorkflowPage
            eyebrow="Relationships"
            title={`Edit ${selected.account.name}`}
            backLabel={`Back to ${selected.account.name}`}
            onBack={() => navigate(`/accounts/${selected.account.id}`)}
          >
            <form onSubmit={updateAccount}>
              <label>
                Business name
                <input
                  name="name"
                  defaultValue={selected.account.name}
                  maxLength={160}
                  required
                  autoFocus
                />
              </label>
              <div className="field-grid">
                <label>
                  Relationship
                  <select name="status" defaultValue={selected.account.status}>
                    <option value="prospect">Prospect</option>
                    <option value="customer">Customer</option>
                    <option value="inactive">Inactive</option>
                  </select>
                </label>
                <label>
                  Billing email
                  <input
                    name="billingEmail"
                    type="email"
                    defaultValue={selected.account.billingEmail}
                  />
                </label>
              </div>
              <fieldset className="form-section">
                <legend>Websites and domains</legend>
                <p className="field-help">
                  Add, correct, or remove every domain tied to this account.
                </p>
                {editWebsiteFields.map((value, index) => (
                  <div className="repeatable-field" key={index}>
                    <label>
                      Website {index + 1}
                      <input
                        value={value}
                        onChange={(event) =>
                          setEditWebsiteFields((current) =>
                            current.map((item, itemIndex) =>
                              itemIndex === index ? event.target.value : item,
                            ),
                          )
                        }
                        placeholder="example.com"
                        inputMode="url"
                      />
                    </label>
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={`Remove website ${index + 1}`}
                      onClick={() =>
                        setEditWebsiteFields((current) =>
                          current.filter((_, itemIndex) => itemIndex !== index),
                        )
                      }
                    >
                      <Trash2 size={17} />
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className="text-button"
                  onClick={() =>
                    setEditWebsiteFields((current) => [...current, ""])
                  }
                >
                  <Plus size={15} /> Add another website
                </button>
              </fieldset>
              <label>
                Notes
                <textarea
                  name="notes"
                  rows={4}
                  defaultValue={selected.account.notes}
                />
              </label>
              {formError && (
                <p className="form-error" role="alert">
                  {formError}
                </p>
              )}
              <div className="form-actions">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => navigate(`/accounts/${selected.account.id}`)}
                >
                  Cancel
                </button>
                <button className="primary-button" disabled={saving}>
                  {saving ? "Saving..." : "Save changes"}
                </button>
              </div>
            </form>
          </WorkflowPage>
        )}
        {route.action === "links" && (
          <WorkflowPage
            eyebrow="Account resources"
            title={`Links for ${selected.account.name}`}
            backLabel={`Back to ${selected.account.name}`}
            onBack={() => navigate(`/accounts/${selected.account.id}`)}
          >
            <form onSubmit={updateLinks}>
              <p className="field-help">
                Keep Sheets, Google Docs, proposals, and other shared resources
                with this account.
              </p>
              {editLinkFields.map((link, index) => (
                <fieldset
                  className="form-section account-link-field"
                  key={index}
                >
                  <legend>Link {index + 1}</legend>
                  <label>
                    Name
                    <input
                      aria-label={`Link ${index + 1} name`}
                      value={link.label}
                      maxLength={120}
                      required
                      onChange={(event) =>
                        setEditLinkFields((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, label: event.target.value }
                              : item,
                          ),
                        )
                      }
                      placeholder="Project spreadsheet"
                    />
                  </label>
                  <label>
                    URL
                    <input
                      aria-label={`Link ${index + 1} URL`}
                      value={link.url}
                      type="url"
                      inputMode="url"
                      required
                      onChange={(event) =>
                        setEditLinkFields((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, url: event.target.value }
                              : item,
                          ),
                        )
                      }
                      placeholder="https://docs.google.com/..."
                    />
                  </label>
                  <button
                    type="button"
                    className="text-button danger-text"
                    onClick={() =>
                      setEditLinkFields((current) =>
                        current.filter((_, itemIndex) => itemIndex !== index),
                      )
                    }
                  >
                    <Trash2 size={15} /> Remove link
                  </button>
                </fieldset>
              ))}
              <button
                type="button"
                className="text-button"
                onClick={() =>
                  setEditLinkFields((current) => [
                    ...current,
                    { label: "", url: "" },
                  ])
                }
              >
                <Plus size={15} /> Add another link
              </button>
              {formError && (
                <p className="form-error" role="alert">
                  {formError}
                </p>
              )}
              <div className="form-actions">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => navigate(`/accounts/${selected.account.id}`)}
                >
                  Cancel
                </button>
                <button className="primary-button" disabled={saving}>
                  {saving ? "Saving..." : "Save links"}
                </button>
              </div>
            </form>
          </WorkflowPage>
        )}
        {route.action === "domain" && (
          <WorkflowPage
            eyebrow="Cloudflare"
            title={`Link a domain to ${selected.account.name}`}
            backLabel={`Back to ${selected.account.name}`}
            onBack={() => navigate(`/accounts/${selected.account.id}`)}
          >
            {loadingDomains ? (
              <LoadingState label="Loading Cloudflare domains" />
            ) : (
              <form onSubmit={linkDomain}>
                <label>
                  Domain
                  <select
                    name="domainName"
                    value={selectedDomain}
                    onChange={(event) => setSelectedDomain(event.target.value)}
                    required
                  >
                    <option value="">Choose a domain</option>
                    {domains.map((item) => (
                      <option value={item.domainName} key={item.domainName}>
                        {item.domainName}
                        {item.registered ? " · Cloudflare Registrar" : ""}
                      </option>
                    ))}
                  </select>
                </label>
                {domains.find((item) => item.domainName === selectedDomain)
                  ?.renewalDate ? (
                  <p className="inline-notice">
                    <span className="security-dot" /> Renews{" "}
                    {
                      domains.find((item) => item.domainName === selectedDomain)
                        ?.renewalDate
                    }
                    . Kosmos will add reminders 30, 14, and 7 days before.
                  </p>
                ) : (
                  <label>
                    Registrar renewal date
                    <input name="renewalDate" type="date" required />
                    <small className="field-help">
                      Cloudflare hosts this zone but does not register it, so
                      its API has no renewal date.
                    </small>
                  </label>
                )}
                {formError && (
                  <p className="form-error" role="alert">
                    {formError}
                  </p>
                )}
                <div className="form-actions">
                  <button
                    type="button"
                    className="secondary-button"
                    onClick={() => navigate(`/accounts/${selected.account.id}`)}
                  >
                    Cancel
                  </button>
                  <button
                    className="primary-button"
                    disabled={saving || !selectedDomain}
                  >
                    {saving ? "Linking..." : "Link domain and reminders"}
                  </button>
                </div>
              </form>
            )}
          </WorkflowPage>
        )}
        {route.action === "delete" && (
          <WorkflowPage
            eyebrow="Relationships"
            title="Delete this account?"
            detail={`${selected.account.name}, its contacts, opportunities, follow-up reminders, activities, and records linked only to this account will be permanently removed. Shared documents keep their other links.`}
            backLabel="Keep account"
            onBack={() => navigate(`/accounts/${selected.account.id}`)}
            tone="danger"
          >
            {formError && (
              <p className="form-error" role="alert">
                {formError}
              </p>
            )}
            <div className="form-actions">
              <button
                className="secondary-button"
                onClick={() => navigate(`/accounts/${selected.account.id}`)}
              >
                Keep account
              </button>
              <button
                className="danger-button"
                onClick={deleteAccount}
                disabled={saving}
              >
                <Trash2 size={16} />
                {saving ? "Deleting..." : "Delete account"}
              </button>
            </div>
          </WorkflowPage>
        )}
      </>
    );

  return (
    <>
      {route.action === "list" && (
        <Page
          eyebrow="Relationships"
          title="Accounts"
          detail="One home for each business, its people, websites, pipeline, and lifecycle."
          action={
            <button
              className="primary-button"
              onClick={() => navigate("/accounts/new")}
            >
              <Plus size={17} /> Add account
            </button>
          }
        >
          {items.length ? (
            <div className="record-grid">
              {items.map((account) => (
                <button
                  className="record-card"
                  key={account.id}
                  onClick={() => navigate(`/accounts/${account.id}`)}
                >
                  <span className="record-avatar">
                    <Building2 size={18} />
                  </span>
                  <span className="record-main">
                    <strong>{account.name}</strong>
                    <small>
                      {account.billingEmail ||
                        accountWebsites(account)[0]?.domain ||
                        "No details yet"}
                    </small>
                  </span>
                  <span className={`status-badge ${account.status}`}>
                    {account.status}
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <EmptyState
              title="No accounts yet"
              detail="Create the first business and its primary contact together."
              action={
                <button
                  className="primary-button"
                  onClick={() => navigate("/accounts/new")}
                >
                  Add your first account
                </button>
              }
            />
          )}
        </Page>
      )}
      {route.action === "new" && (
        <WorkflowPage
          eyebrow="Relationships"
          title="Add an account"
          detail="Create the business and, optionally, its first contact in one step."
          backLabel="Back to accounts"
          onBack={() => {
            resetCreateForm();
            navigate("/accounts");
          }}
        >
          <form onSubmit={create}>
            <label>
              Business name
              <input name="name" maxLength={160} required autoFocus />
            </label>
            <div className="field-grid">
              <label>
                Relationship
                <select name="status" defaultValue="prospect">
                  <option value="prospect">Prospect</option>
                  <option value="customer">Customer</option>
                  <option value="inactive">Inactive</option>
                </select>
              </label>
              <label>
                Billing email
                <input name="billingEmail" type="email" />
              </label>
            </div>
            <fieldset className="form-section">
              <legend>Websites</legend>
              <p className="field-help">
                Add every website this business owns. Connect a domain to
                Cloudflare after saving.
              </p>
              {websiteFields.map((value, index) => (
                <div className="repeatable-field" key={index}>
                  <label>
                    Website {index + 1}
                    <input
                      value={value}
                      onChange={(event) =>
                        setWebsiteFields((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index ? event.target.value : item,
                          ),
                        )
                      }
                      placeholder="example.com"
                      inputMode="url"
                    />
                  </label>
                  {websiteFields.length > 1 && (
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={`Remove website ${index + 1}`}
                      onClick={() =>
                        setWebsiteFields((current) =>
                          current.filter((_, itemIndex) => itemIndex !== index),
                        )
                      }
                    >
                      <Trash2 size={17} />
                    </button>
                  )}
                </div>
              ))}
              <button
                type="button"
                className="text-button"
                onClick={() => setWebsiteFields((current) => [...current, ""])}
              >
                <Plus size={15} /> Add another website
              </button>
            </fieldset>
            <fieldset className="form-section contact-section">
              <legend>Primary contact</legend>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={includeContact}
                  onChange={(event) => setIncludeContact(event.target.checked)}
                />{" "}
                Create their first contact now
              </label>
              {includeContact && (
                <div className="field-grid contact-fields">
                  <label>
                    Full name
                    <input name="contactName" maxLength={160} required />
                  </label>
                  <label>
                    Email
                    <input name="contactEmail" type="email" />
                  </label>
                  <label>
                    Phone
                    <input name="contactPhone" type="tel" />
                  </label>
                  <label>
                    LinkedIn profile
                    <input
                      name="contactLinkedInUrl"
                      type="url"
                      inputMode="url"
                      placeholder="https://www.linkedin.com/in/name"
                    />
                  </label>
                  <ContactSourcePicker name="contactSource" />
                </div>
              )}
            </fieldset>
            <label>
              Notes
              <textarea name="notes" rows={4} />
            </label>
            {formError && (
              <p className="form-error" role="alert">
                {formError}
              </p>
            )}
            <div className="form-actions">
              <button
                type="button"
                className="secondary-button"
                onClick={() => {
                  resetCreateForm();
                  navigate("/accounts");
                }}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving ? "Saving account and contact..." : "Save account"}
              </button>
            </div>
          </form>
        </WorkflowPage>
      )}
    </>
  );
}

function AccountView({
  detail,
  events,
  cloudflare,
  navigate,
  onLink,
  saving,
  formError,
  onStatus,
  onEdit,
  onManageLinks,
  onDelete,
}: {
  detail: AccountDetail;
  events: AccountEvent[];
  cloudflare: CloudflareStatus | null;
  navigate: (path: string) => void;
  onLink: () => void;
  saving: boolean;
  formError: string;
  onStatus: (status: Account["status"]) => void;
  onEdit: () => void;
  onManageLinks: () => void;
  onDelete: () => void;
}) {
  const websites = accountWebsites(detail.account);
  const documents = sortedDocuments(detail.documents || []);

  return (
    <div className="page">
      <button className="back-button" onClick={() => navigate("/accounts")}>
        ← All accounts
      </button>
      <header className="account-hero">
        <RecordPhoto
          recordType="account"
          recordID={detail.account.id}
          label={detail.account.name}
          fallback={<Building2 size={26} />}
        />
        <div>
          <label className="account-status-control">
            Relationship
            <select
              aria-label="Account relationship"
              value={detail.account.status}
              disabled={saving}
              onChange={(event) =>
                onStatus(event.target.value as Account["status"])
              }
            >
              <option value="prospect">Prospect</option>
              <option value="customer">Customer</option>
              <option value="inactive">Inactive</option>
            </select>
          </label>
          <h1>{detail.account.name}</h1>
          <p className="subhead">
            {detail.account.billingEmail || "No billing email yet"}
          </p>
          {formError && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
        </div>
        <div className="button-row account-hero-action">
          <button className="secondary-button" onClick={onEdit}>
            <Pencil size={16} /> Edit account
          </button>
          <button className="danger-button" onClick={onDelete}>
            <Trash2 size={16} /> Delete account
          </button>
        </div>
      </header>
      <section className="stats-row">
        <div className="stat-card blue">
          <span className="stat-label">Contacts</span>
          <strong className="stat-value">{detail.contacts.length}</strong>
        </div>
        <div className="stat-card gold">
          <span className="stat-label">Open opportunities</span>
          <strong className="stat-value">
            {
              detail.opportunities.filter(
                (item) => !["won", "lost"].includes(item.stage),
              ).length
            }
          </strong>
        </div>
        <div className="stat-card green">
          <span className="stat-label">Pipeline</span>
          <strong className="stat-value">
            {money(
              detail.opportunities.reduce(
                (sum, item) =>
                  ["won", "lost"].includes(item.stage)
                    ? sum
                    : sum + item.amountCents,
                0,
              ),
            )}
          </strong>
        </div>
      </section>
      <section className="panel account-websites">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Online</p>
            <h2>Websites and domains</h2>
          </div>
          {cloudflare?.connected ? (
            <button className="secondary-button" onClick={onLink}>
              <Cloud size={16} /> Link Cloudflare domain
            </button>
          ) : (
            <button
              className="secondary-button"
              onClick={() => navigate("/settings")}
            >
              <Cloud size={16} /> Connect Cloudflare
            </button>
          )}
        </div>
        {websites.length ? (
          <div className="website-list">
            {websites.map((website) => (
              <a
                href={website.url}
                target="_blank"
                rel="noreferrer"
                key={`${website.domain}-${website.url}`}
              >
                <span className="setting-icon">
                  <Globe2 size={18} />
                </span>
                <span>
                  <strong>{website.domain || website.url}</strong>
                  <small>
                    {website.provider === "cloudflare"
                      ? `Cloudflare${website.renewalDate ? ` · renews ${website.renewalDate}` : ""}`
                      : website.url}
                  </small>
                </span>
                <ExternalLink size={15} />
              </a>
            ))}
          </div>
        ) : (
          <p className="muted-copy">No websites linked yet.</p>
        )}
      </section>
      <section className="panel account-links">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Resources</p>
            <h2>Links</h2>
          </div>
          <button className="secondary-button" onClick={onManageLinks}>
            <Link2 size={16} /> Manage links
          </button>
        </div>
        {detail.account.links?.length ? (
          <div className="account-link-list">
            {detail.account.links.map((link) => (
              <a
                href={link.url}
                target="_blank"
                rel="noreferrer"
                key={`${link.label}-${link.url}`}
              >
                <span className="setting-icon">
                  <Link2 size={18} />
                </span>
                <span>
                  <strong>{link.label}</strong>
                  <small>{link.url}</small>
                </span>
                <ExternalLink size={15} />
              </a>
            ))}
          </div>
        ) : (
          <p className="muted-copy">
            No links yet. Add a Sheet, Google Doc, proposal, or shared folder.
          </p>
        )}
      </section>
      <section className="split-grid">
        <div className="panel">
          <p className="eyebrow">People</p>
          <h2>Account contacts</h2>
          {detail.contacts.length ? (
            detail.contacts.map((contact) => (
              <button
                className="record-row compact row-button"
                onClick={() => navigate(`/contacts/${contact.id}`)}
                key={contact.id}
              >
                <strong>{contact.name}</strong>
                <small>{contact.email}</small>
              </button>
            ))
          ) : (
            <p className="muted-copy">Add a contact and choose this account.</p>
          )}
        </div>
        <div className="panel">
          <p className="eyebrow">Pipeline</p>
          <h2>Opportunities</h2>
          {detail.opportunities.length ? (
            detail.opportunities.map((item) => (
              <button
                className="record-row compact row-button"
                onClick={() =>
                  navigate(`/opportunities?account=${detail.account.id}`)
                }
                key={item.id}
              >
                <span>
                  <strong>{item.name}</strong>
                  <small>{item.stage}</small>
                </span>
                <strong>{money(item.amountCents)}</strong>
              </button>
            ))
          ) : (
            <p className="muted-copy">No opportunities are linked yet.</p>
          )}
        </div>
      </section>
      <section className="panel account-events">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Timeline</p>
            <h2>Recent events</h2>
          </div>
          <button
            className="secondary-button"
            onClick={() => navigate(`/accounts/${detail.account.id}/events`)}
          >
            View all events
          </button>
        </div>
        {events.length ? (
          <AccountEventList events={events} />
        ) : (
          <p className="muted-copy">
            New account changes and customer activity will appear here.
          </p>
        )}
      </section>
      <section className="panel account-documents">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Knowledge</p>
            <h2>Documents</h2>
          </div>
          <button
            className="secondary-button"
            onClick={() =>
              navigate(`/documents/new?account=${detail.account.id}`)
            }
          >
            <FilePlus2 size={16} /> New document
          </button>
        </div>
        {documents.length ? (
          <div className="document-records">
            {documents.map((document) => (
              <button
                className="record-row row-button"
                onClick={() => navigate(`/documents/${document.id}`)}
                key={document.id}
              >
                <span className="setting-icon">
                  <FileText size={17} />
                </span>
                <span className="record-main">
                  <strong>{document.title}</strong>
                  <small>
                    Added {new Date(document.createdAt).toLocaleDateString()}
                  </small>
                </span>
              </button>
            ))}
          </div>
        ) : (
          <p className="muted-copy">
            No documents yet. Add notes, plans, or customer details here.
          </p>
        )}
      </section>
    </div>
  );
}

const eventKinds = [
  ["", "Everything"],
  ["account", "Account"],
  ["contact", "Contacts"],
  ["opportunity", "Opportunities"],
  ["email", "Email"],
  ["call", "Calls"],
  ["text", "Texts"],
  ["activity", "Notes and meetings"],
  ["reminder", "Reminders"],
  ["document", "Documents"],
  ["domain", "Domains"],
  ["transaction", "Transactions"],
] as const;

function AccountEventsPage({
  account,
  navigate,
}: {
  account: Account;
  navigate: (path: string) => void;
}) {
  const [events, setEvents] = useState<AccountEvent[]>([]);
  const [page, setPage] = useState<PageMetadata>({});
  const [kind, setKind] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadEvents = useCallback(
    async (cursor = "", append = false) => {
      setLoading(true);
      setError("");
      const query = new URLSearchParams({ limit: "20" });
      if (kind) query.set("kind", kind);
      if (cursor) query.set("cursor", cursor);
      try {
        const response = await apiPage<{
          events: AccountEvent[];
          page: PageMetadata;
        }>(`/api/v1/accounts/${account.id}/events?${query}`);
        setEvents((current) =>
          append ? [...current, ...response.events] : response.events,
        );
        setPage(response.page);
      } catch (reason) {
        setError(
          reason instanceof Error ? reason.message : "Could not load events",
        );
      } finally {
        setLoading(false);
      }
    },
    [account.id, kind],
  );

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

  return (
    <Page
      eyebrow="Account history"
      title={`${account.name} events`}
      detail="Every change and customer interaction Kosmos can tie to this account."
      action={
        <button
          className="secondary-button"
          onClick={() => navigate(`/accounts/${account.id}`)}
        >
          ← Back to {account.name}
        </button>
      }
    >
        <section className="panel">
          <div className="event-toolbar">
            <label>
              Show
              <select value={kind} onChange={(event) => setKind(event.target.value)}>
                {eventKinds.map(([value, label]) => (
                  <option value={value} key={value || "all"}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <span>{events.length} shown</span>
          </div>
          {error ? (
            <ErrorState message={error} retry={() => loadEvents()} />
          ) : loading && !events.length ? (
            <LoadingState label="Loading account events" />
          ) : events.length ? (
            <>
              <AccountEventList events={events} />
              <div className="form-actions event-actions">
                {events.length > 20 && (
                  <button
                    className="secondary-button"
                    onClick={() => loadEvents()}
                    disabled={loading}
                  >
                    Show less
                  </button>
                )}
                {page.nextCursor && (
                  <button
                    className="primary-button"
                    onClick={() => loadEvents(page.nextCursor, true)}
                    disabled={loading}
                  >
                    {loading ? "Loading..." : "Load more"}
                  </button>
                )}
              </div>
            </>
          ) : (
            <EmptyState
              title="No matching events"
              detail="Try another event type or come back after more account activity."
            />
          )}
        </section>
    </Page>
  );
}

function AccountEventList({ events }: { events: AccountEvent[] }) {
  return (
    <div className="account-event-list">
      {events.map((event) => (
        <article className="account-event" key={event.id}>
          <span className="activity-icon lavender">
            <Clock3 size={16} />
          </span>
          <span className="account-event-copy">
            <strong>{event.title}</strong>
            {event.summary && <small>{event.summary}</small>}
            <time dateTime={event.occurredAt}>
              {new Date(event.occurredAt).toLocaleString()} · {event.actor}
            </time>
          </span>
          <span className="event-kind">{event.kind}</span>
        </article>
      ))}
    </div>
  );
}

function sortedDocuments(documents: Document[]) {
  return [...documents].sort(
    (left, right) =>
      new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime(),
  );
}

function accountWebsites(account: Account): Website[] {
  if (account.websites?.length) return account.websites;
  if (account.website) {
    try {
      return [
        {
          url: account.website,
          domain: new URL(account.website).hostname,
          autoRenew: false,
        },
      ];
    } catch {
      return [
        { url: account.website, domain: account.website, autoRenew: false },
      ];
    }
  }
  return [];
}
