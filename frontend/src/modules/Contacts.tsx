import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  Building2,
  CalendarPlus,
  ContactRound,
  ExternalLink,
  Mail,
  MessageSquarePlus,
  Pencil,
  Phone,
  Plus,
  Trash2,
} from "lucide-react";
import {
  Account,
  Activity,
  api,
  Contact,
  Opportunity,
  Reminder,
  shortDate,
} from "../api";
import { ContactSourcePicker } from "../components/ContactSourcePicker";
import { GoogleVoiceButton } from "../components/GoogleVoiceButton";
import { RecordPhoto } from "../components/RecordPhoto";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { WorkflowPage } from "../components/WorkflowPage";
import { ResourceRoute } from "../routing";

export function Contacts({
  route,
  navigate,
}: {
  route: ResourceRoute;
  navigate: (path: string) => void;
}) {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [actionError, setActionError] = useState("");
  const [saving, setSaving] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setError("");
    Promise.all([
      api<{ contacts: Contact[] }>("/api/v1/contacts"),
      api<{ activities: Activity[] }>("/api/v1/activities"),
      api<{ reminders: Reminder[] }>("/api/v1/reminders"),
      api<{ opportunities: Opportunity[] }>("/api/v1/opportunities"),
      api<{ accounts: Account[] }>("/api/v1/accounts"),
    ])
      .then(
        ([
          contactResponse,
          activityResponse,
          reminderResponse,
          opportunityResponse,
          accountResponse,
        ]) => {
          setContacts(contactResponse.contacts);
          setActivities(activityResponse.activities);
          setReminders(reminderResponse.reminders);
          setOpportunities(opportunityResponse.opportunities);
          setAccounts(accountResponse.accounts);
        },
      )
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(load, [load]);
  const selected = contacts.find((contact) => contact.id === route.id);

  async function createContact(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    try {
      const contact = await api<Contact>("/api/v1/contacts", {
        method: "POST",
        body: JSON.stringify({
          name: form.get("name"),
          email: form.get("email"),
          phone: form.get("phone"),
          linkedinUrl: form.get("linkedinUrl"),
          accountId: form.get("accountId"),
          source: form.get("source"),
        }),
      });
      setContacts((current) => [contact, ...current]);
      navigate(`/contacts/${contact.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save contact",
      );
    } finally {
      setSaving(false);
    }
  }

  async function updateContact(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    try {
      const contact = await api<Contact>(`/api/v1/contacts/${selected.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          name: form.get("name"),
          email: form.get("email"),
          phone: form.get("phone"),
          linkedinUrl: form.get("linkedinUrl"),
          accountId: form.get("accountId"),
          source: form.get("source"),
        }),
      });
      setContacts((current) =>
        current.map((item) => (item.id === contact.id ? contact : item)),
      );
      navigate(`/contacts/${contact.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not update contact",
      );
    } finally {
      setSaving(false);
    }
  }

  async function addActivity(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    setActionError("");
    try {
      const created = await api<Activity>("/api/v1/activities", {
        method: "POST",
        body: JSON.stringify({
          contactId: selected.id,
          kind: data.get("kind"),
          body: data.get("body"),
        }),
      });
      setActivities((current) => [created, ...current]);
      form.reset();
    } catch (reason) {
      setActionError(
        reason instanceof Error ? reason.message : "Could not add activity",
      );
    }
  }

  async function deleteContact() {
    if (!selected) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/contacts/${selected.id}`, { method: "DELETE" });
      setContacts((current) =>
        current.filter((contact) => contact.id !== selected.id),
      );
      navigate("/contacts");
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not delete contact",
      );
    } finally {
      setSaving(false);
    }
  }

  async function addReminder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    setActionError("");
    try {
      const created = await api<Reminder>("/api/v1/reminders", {
        method: "POST",
        body: JSON.stringify({
          contactId: selected.id,
          title: data.get("title"),
          dueAt: new Date(String(data.get("dueAt"))).toISOString(),
        }),
      });
      setReminders((current) => [...current, created]);
      form.reset();
    } catch (reason) {
      setActionError(
        reason instanceof Error ? reason.message : "Could not add reminder",
      );
    }
  }

  if (loading) return <LoadingState label="Loading your people" />;
  if (error) return <ErrorState message={error} retry={load} />;

  if (route.action === "new")
    return (
      <WorkflowPage
        eyebrow="Relationships"
        title="Add a contact"
        detail="Capture the person and connect them to their account in one pass."
        backLabel="All contacts"
        onBack={() => navigate("/contacts")}
      >
        <ContactForm
          accounts={accounts}
          saving={saving}
          error={formError}
          submitLabel="Save contact"
          onSubmit={createContact}
          onCancel={() => navigate("/contacts")}
        />
      </WorkflowPage>
    );

  if ((route.action === "edit" || route.action === "delete") && !selected)
    return <ErrorState message="That contact could not be found." />;

  if (route.action === "edit" && selected)
    return (
      <WorkflowPage
        eyebrow="Relationships"
        title={`Edit ${selected.name}`}
        detail="Update this person without losing the rest of their account history."
        backLabel="Back to contact"
        onBack={() => navigate(`/contacts/${selected.id}`)}
      >
        <ContactForm
          contact={selected}
          accounts={accounts}
          saving={saving}
          error={formError}
          submitLabel="Save changes"
          onSubmit={updateContact}
          onCancel={() => navigate(`/contacts/${selected.id}`)}
        />
      </WorkflowPage>
    );

  if (route.action === "delete" && selected)
    return (
      <WorkflowPage
        eyebrow="Relationships"
        title="Delete this contact?"
        detail={`${selected.name} will be permanently removed from Kosmos and from the connected shared Google Contacts account after its queued sync completes.`}
        backLabel="Keep contact"
        onBack={() => navigate(`/contacts/${selected.id}`)}
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
            onClick={() => navigate(`/contacts/${selected.id}`)}
          >
            Keep contact
          </button>
          <button
            className="danger-button"
            onClick={deleteContact}
            disabled={saving}
          >
            <Trash2 size={16} /> {saving ? "Deleting..." : "Delete contact"}
          </button>
        </div>
      </WorkflowPage>
    );

  if (selected)
    return (
      <ContactAccount
        contact={selected}
        account={accounts.find((account) => account.id === selected.accountId)}
        activities={activities.filter((item) => item.contactId === selected.id)}
        reminders={reminders.filter((item) => item.contactId === selected.id)}
        opportunities={opportunities.filter(
          (item) => item.contactId === selected.id,
        )}
        actionError={actionError}
        onBack={() => navigate("/contacts")}
        onEdit={() => navigate(`/contacts/${selected.id}/edit`)}
        onDelete={() => navigate(`/contacts/${selected.id}/delete`)}
        onActivity={addActivity}
        onReminder={addReminder}
      />
    );

  return (
    <Page
      eyebrow="Relationships"
      title="Contacts"
      detail="The people you talk to, linked to the businesses and opportunities they belong to."
      action={
        <div className="button-row page-actions">
          <button
            className="secondary-button"
            onClick={() => navigate("/lead")}
          >
            <ContactRound size={17} /> Quick lead
          </button>
          <button
            className="primary-button"
            onClick={() => navigate("/contacts/new")}
          >
            <Plus size={17} /> Add contact
          </button>
        </div>
      }
    >
      {contacts.length ? (
        <div className="record-grid">
          {contacts.map((contact) => (
            <button
              className="record-card"
              key={contact.id}
              onClick={() => navigate(`/contacts/${contact.id}`)}
            >
              <span className="record-avatar">{initials(contact.name)}</span>
              <span className="record-main">
                <strong>{contact.name}</strong>
                <small>
                  {accounts.find((account) => account.id === contact.accountId)
                    ?.name ||
                    contact.email ||
                    "No account yet"}
                </small>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <EmptyState
          title="No people yet"
          detail="Add the first person you want to follow up with."
          action={
            <button
              className="primary-button"
              onClick={() => navigate("/contacts/new")}
            >
              <Plus size={17} /> Add your first contact
            </button>
          }
        />
      )}
    </Page>
  );
}

function ContactAccount({
  contact,
  account,
  activities,
  reminders,
  opportunities,
  actionError,
  onBack,
  onEdit,
  onDelete,
  onActivity,
  onReminder,
}: {
  contact: Contact;
  account?: Account;
  activities: Activity[];
  reminders: Reminder[];
  opportunities: Opportunity[];
  actionError: string;
  onBack: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onActivity: (event: FormEvent<HTMLFormElement>) => void;
  onReminder: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const openReminders = reminders.filter((item) => !item.completed);
  const latest = useMemo(
    () =>
      [...activities].sort(
        (a, b) =>
          new Date(b.occurredAt).getTime() - new Date(a.occurredAt).getTime(),
      ),
    [activities],
  );
  return (
    <div className="page">
      <button className="back-button" onClick={onBack}>
        <ArrowLeft size={16} /> All contacts
      </button>
      <header className="account-hero">
        <RecordPhoto
          recordType="contact"
          recordID={contact.id}
          label={contact.name}
          fallback={initials(contact.name)}
        />
        <div>
          <p className="eyebrow">Contact</p>
          <h1>{contact.name}</h1>
          <p className="subhead">{account?.name || "No account linked"}</p>
        </div>
        <div className="button-row account-hero-action">
          <button className="secondary-button" onClick={onEdit}>
            <Pencil size={16} /> Edit
          </button>
          <button className="danger-button" onClick={onDelete}>
            <Trash2 size={16} /> Delete
          </button>
        </div>
      </header>
      {actionError && (
        <p className="form-error action-error" role="alert">
          {actionError}
        </p>
      )}
      <section className="account-facts">
        {account && (
          <span>
            <Building2 size={16} />
            <small>Account</small>
            <strong>{account.name}</strong>
          </span>
        )}
        {contact.email && (
          <a href={`mailto:${contact.email}`}>
            <Mail size={16} />
            <small>Email</small>
            <strong>{contact.email}</strong>
          </a>
        )}
        {contact.phone && (
          <a href={`tel:${contact.phone}`}>
            <Phone size={16} />
            <small>Phone</small>
            <strong>{contact.phone}</strong>
          </a>
        )}
        {contact.linkedinUrl && (
          <a href={contact.linkedinUrl} target="_blank" rel="noreferrer">
            <ContactRound size={16} />
            <small>LinkedIn</small>
            <strong>Open profile</strong>
          </a>
        )}
        {contact.phone && (
          <span className="account-fact-action">
            <ExternalLink size={16} />
            <small>Google Voice</small>
            <GoogleVoiceButton phone={contact.phone} contactId={contact.id} />
          </span>
        )}
      </section>
      <div className="account-grid">
        <section className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">History</p>
              <h2>Activity and notes</h2>
            </div>
          </div>
          <form className="quick-entry" onSubmit={onActivity}>
            <select name="kind" aria-label="Activity type">
              <option value="note">Note</option>
              <option value="call">Call</option>
              <option value="text">Text</option>
              <option value="email">Email</option>
              <option value="meeting">Meeting</option>
            </select>
            <textarea
              name="body"
              aria-label="Activity note"
              placeholder="What happened?"
              maxLength={4000}
              required
            />
            <button className="primary-button">
              <MessageSquarePlus size={16} /> Add to timeline
            </button>
          </form>
          <div className="timeline">
            {latest.length ? (
              latest.map((item) => (
                <div className="timeline-item" key={item.id}>
                  <span className="timeline-dot" />
                  <div>
                    <strong>{item.kind}</strong>
                    <p>{item.body}</p>
                    <time>{shortDate(item.occurredAt)}</time>
                  </div>
                </div>
              ))
            ) : (
              <p className="quiet-copy">
                No notes yet. Capture the first conversation above.
              </p>
            )}
          </div>
        </section>
        <div className="account-side">
          <section className="panel">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">Next</p>
                <h2>Follow-ups</h2>
              </div>
            </div>
            <form className="stack-form" onSubmit={onReminder}>
              <label>
                What needs to happen?
                <input name="title" maxLength={160} required />
              </label>
              <label>
                When?
                <input name="dueAt" type="datetime-local" required />
              </label>
              <button className="secondary-button">
                <CalendarPlus size={16} /> Add reminder
              </button>
            </form>
            {openReminders.length ? (
              <ul className="simple-list">
                {openReminders.map((item) => (
                  <li key={item.id}>
                    <strong>{item.title}</strong>
                    <small>{shortDate(item.dueAt)}</small>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="quiet-copy">Nothing waiting on you.</p>
            )}
          </section>
          <section className="panel">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">Pipeline</p>
                <h2>Opportunities</h2>
              </div>
            </div>
            {opportunities.length ? (
              <ul className="simple-list">
                {opportunities.map((item) => (
                  <li key={item.id}>
                    <strong>{item.name}</strong>
                    <small>{item.stage}</small>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="quiet-copy">No opportunities linked yet.</p>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

function ContactForm({
  contact,
  accounts,
  saving,
  error,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  contact?: Contact;
  accounts: Account[];
  saving: boolean;
  error: string;
  submitLabel: string;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onCancel: () => void;
}) {
  return (
    <form onSubmit={onSubmit}>
      <div className="field-grid">
        <label>
          Full name
          <input
            name="name"
            maxLength={160}
            defaultValue={contact?.name}
            required
            autoFocus
          />
        </label>
        <label>
          Account
          <select name="accountId" defaultValue={contact?.accountId || ""}>
            <option value="">No account yet</option>
            {accounts.map((account) => (
              <option key={account.id} value={account.id}>
                {account.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Email
          <input name="email" type="email" defaultValue={contact?.email} />
        </label>
        <label>
          Phone
          <input name="phone" type="tel" defaultValue={contact?.phone} />
        </label>
        <label className="field-span">
          LinkedIn profile
          <input
            name="linkedinUrl"
            type="url"
            inputMode="url"
            placeholder="https://www.linkedin.com/in/name"
            defaultValue={contact?.linkedinUrl}
          />
        </label>
        <ContactSourcePicker defaultValue={contact?.source} />
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="form-actions">
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancel
        </button>
        <button className="primary-button" disabled={saving}>
          {saving ? "Saving..." : submitLabel}
        </button>
      </div>
    </form>
  );
}

function initials(name: string) {
  return name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}
