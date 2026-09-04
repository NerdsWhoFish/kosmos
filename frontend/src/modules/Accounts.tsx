import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  Building2,
  Cloud,
  ExternalLink,
  FilePlus2,
  FileText,
  Globe2,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import {
  Account,
  api,
  CloudflareDomain,
  CloudflareStatus,
  Contact,
  Document,
  money,
  Opportunity,
  Website,
} from "../api";
import { Modal } from "../components/Modal";
import { ContactSourcePicker } from "../components/ContactSourcePicker";
import { RecordPhoto } from "../components/RecordPhoto";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";

type AccountDetail = {
  account: Account;
  contacts: Contact[];
  opportunities: Opportunity[];
  documents: Document[];
};
type AccountCreation = { account: Account; contact?: Contact };

export function Accounts({
  initialID,
  navigate,
}: {
  initialID: string;
  navigate: (path: string) => void;
}) {
  const [items, setItems] = useState<Account[]>([]);
  const [selected, setSelected] = useState<AccountDetail | null>(null);
  const [cloudflare, setCloudflare] = useState<CloudflareStatus | null>(null);
  const [domains, setDomains] = useState<CloudflareDomain[]>([]);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState(false);
  const [linking, setLinking] = useState(false);
  const [websiteFields, setWebsiteFields] = useState([""]);
  const [editWebsiteFields, setEditWebsiteFields] = useState([""]);
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
        .then(setSelected)
        .catch((reason: Error) => setError(reason.message)),
    [],
  );

  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (!initialID) {
      setSelected(null);
      return;
    }
    void loadSelected(initialID);
  }, [initialID, loadSelected]);

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
      setCreating(false);
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

  async function openDomainLink() {
    setLinking(true);
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
      setLinking(false);
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

  function openEdit() {
    if (!selected) return;
    setEditWebsiteFields(
      accountWebsites(selected.account)
        .map((website) => website.url)
        .concat(accountWebsites(selected.account).length ? [] : [""]),
    );
    setFormError("");
    setEditing(true);
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
      setEditing(false);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not update account",
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
        <AccountView
          detail={selected}
          cloudflare={cloudflare}
          navigate={navigate}
          onLink={openDomainLink}
          linking={linking}
          domains={domains}
          selectedDomain={selectedDomain}
          setSelectedDomain={setSelectedDomain}
          loadingDomains={loadingDomains}
          saving={saving}
          formError={formError}
          onStatus={updateStatus}
          onSubmitDomain={linkDomain}
          onCloseDomain={() => setLinking(false)}
          onEdit={openEdit}
        />
        {editing && (
          <Modal
            eyebrow="Relationships"
            title={`Edit ${selected.account.name}`}
            onClose={() => setEditing(false)}
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
                  onClick={() => setEditing(false)}
                >
                  Cancel
                </button>
                <button className="primary-button" disabled={saving}>
                  {saving ? "Saving..." : "Save changes"}
                </button>
              </div>
            </form>
          </Modal>
        )}
      </>
    );

  return (
    <>
      <Page
        eyebrow="Relationships"
        title="Accounts"
        detail="One home for each business, its people, websites, pipeline, and lifecycle."
        action={
          <button className="primary-button" onClick={() => setCreating(true)}>
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
                onClick={() => setCreating(true)}
              >
                Add your first account
              </button>
            }
          />
        )}
      </Page>
      {creating && (
        <Modal
          eyebrow="Relationships"
          title="Add an account"
          onClose={() => {
            setCreating(false);
            resetCreateForm();
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
                  setCreating(false);
                  resetCreateForm();
                }}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving ? "Saving account and contact..." : "Save account"}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

function AccountView({
  detail,
  cloudflare,
  navigate,
  onLink,
  linking,
  domains,
  selectedDomain,
  setSelectedDomain,
  loadingDomains,
  saving,
  formError,
  onStatus,
  onSubmitDomain,
  onCloseDomain,
  onEdit,
}: {
  detail: AccountDetail;
  cloudflare: CloudflareStatus | null;
  navigate: (path: string) => void;
  onLink: () => void;
  linking: boolean;
  domains: CloudflareDomain[];
  selectedDomain: string;
  setSelectedDomain: (value: string) => void;
  loadingDomains: boolean;
  saving: boolean;
  formError: string;
  onStatus: (status: Account["status"]) => void;
  onSubmitDomain: (event: FormEvent<HTMLFormElement>) => void;
  onCloseDomain: () => void;
  onEdit: () => void;
}) {
  const websites = accountWebsites(detail.account);
  const domain = useMemo(
    () => domains.find((item) => item.domainName === selectedDomain),
    [domains, selectedDomain],
  );
  const [documents, setDocuments] = useState(() =>
    sortedDocuments(detail.documents || []),
  );
  const [creatingDocument, setCreatingDocument] = useState(false);
  const [documentError, setDocumentError] = useState("");
  const [savingDocument, setSavingDocument] = useState(false);

  useEffect(
    () => setDocuments(sortedDocuments(detail.documents || [])),
    [detail.account.id, detail.documents],
  );

  async function createDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSavingDocument(true);
    setDocumentError("");
    const form = new FormData(event.currentTarget);
    try {
      const document = await api<Document>("/api/v1/documents", {
        method: "POST",
        body: JSON.stringify({
          title: form.get("title"),
          body: form.get("body"),
          links: [{ type: "account", id: detail.account.id }],
        }),
      });
      setDocuments((current) => sortedDocuments([document, ...current]));
      setCreatingDocument(false);
      navigate(`/documents/${document.id}`);
    } catch (reason) {
      setDocumentError(
        reason instanceof Error ? reason.message : "Could not create document",
      );
    } finally {
      setSavingDocument(false);
    }
  }

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
          {formError && !linking && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
        </div>
        <button
          className="secondary-button account-hero-action"
          onClick={onEdit}
        >
          <Pencil size={16} /> Edit account
        </button>
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
      <section className="panel account-documents">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Knowledge</p>
            <h2>Documents</h2>
          </div>
          <button
            className="secondary-button"
            onClick={() => setCreatingDocument(true)}
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
      {linking && (
        <Modal
          eyebrow="Cloudflare"
          title="Link a domain"
          onClose={onCloseDomain}
        >
          {loadingDomains ? (
            <LoadingState label="Loading Cloudflare domains" />
          ) : (
            <form onSubmit={onSubmitDomain}>
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
              {domain?.renewalDate ? (
                <p className="inline-notice">
                  <span className="security-dot" /> Renews {domain.renewalDate}.
                  Kosmos will add reminders 30, 14, and 7 days before.
                </p>
              ) : (
                <label>
                  Registrar renewal date
                  <input name="renewalDate" type="date" required />
                  <small className="field-help">
                    Cloudflare hosts this zone but does not register it, so its
                    API has no renewal date.
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
                  onClick={onCloseDomain}
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
        </Modal>
      )}
      {creatingDocument && (
        <Modal
          eyebrow="Knowledge"
          title={`New document for ${detail.account.name}`}
          onClose={() => setCreatingDocument(false)}
        >
          <form onSubmit={createDocument}>
            <label>
              Title
              <input name="title" maxLength={160} required autoFocus />
            </label>
            <label>
              Start writing
              <textarea
                name="body"
                rows={10}
                maxLength={100000}
                placeholder="Markdown is supported."
              />
            </label>
            {documentError && (
              <p className="form-error" role="alert">
                {documentError}
              </p>
            )}
            <div className="form-actions">
              <button
                type="button"
                className="secondary-button"
                onClick={() => setCreatingDocument(false)}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={savingDocument}>
                {savingDocument ? "Creating..." : "Create document"}
              </button>
            </div>
          </form>
        </Modal>
      )}
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
