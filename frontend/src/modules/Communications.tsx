import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  ExternalLink,
  Mail,
  MessageSquareText,
  Plus,
  RefreshCw,
  Send,
} from "lucide-react";
import {
  AcceptedJob,
  Account,
  api,
  Contact,
  EmailTemplate,
  GoogleStatus,
  MailMessage,
  Notification,
  shortDate,
} from "../api";
import { Modal } from "../components/Modal";
import { GoogleVoiceButton } from "../components/GoogleVoiceButton";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";

export function mergeTemplate(
  value: string,
  contact?: Contact,
  account?: Account,
) {
  return value
    .replaceAll("{{name}}", contact?.name || "{{name}}")
    .replaceAll("{{company}}", account?.name || "{{company}}")
    .replaceAll(
      "{{domains}}",
      account?.websites
        ?.map((website) => website.domain || website.url)
        .filter(Boolean)
        .join(", ") || "{{domains}}",
    );
}

export function hasTemplateVariables(value: string) {
  return /{{(?:name|company|domains)}}/.test(value);
}

export function Communications() {
  const [status, setStatus] = useState<GoogleStatus | null>(null);
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [templateOpen, setTemplateOpen] = useState(false);
  const [sending, setSending] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [phone, setPhone] = useState("");
  const [voiceContactID, setVoiceContactID] = useState("");
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [draft, setDraft] = useState({ to: "", subject: "", body: "" });
  const [activeTemplate, setActiveTemplate] = useState<EmailTemplate | null>(
    null,
  );

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      api<GoogleStatus>("/api/v1/integrations/google"),
      api<{ templates: EmailTemplate[] }>("/api/v1/email/templates"),
      api<{ messages: MailMessage[] }>("/api/v1/email/messages"),
      api<{ notifications: Notification[] }>("/api/v1/notifications"),
      api<{ contacts: Contact[] }>("/api/v1/contacts"),
      api<{ accounts: Account[] }>("/api/v1/accounts"),
    ])
      .then(
        ([
          connection,
          templateResult,
          messageResult,
          notificationResult,
          contactResult,
          accountResult,
        ]) => {
          setStatus(connection);
          setTemplates(templateResult.templates);
          setMessages(messageResult.messages);
          setNotifications(notificationResult.notifications);
          setContacts(contactResult.contacts);
          setAccounts(accountResult.accounts);
          setError("");
        },
      )
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(load, [load]);

  async function sendEmail(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (hasTemplateVariables(`${draft.subject}\n${draft.body}`)) {
      setNotice(
        "Choose a known contact or replace the remaining template variables before sending.",
      );
      return;
    }
    const formElement = event.currentTarget;
    setSending(true);
    setNotice("");
    const form = new FormData(event.currentTarget);
    try {
      await api("/api/v1/email/send", {
        method: "POST",
        body: JSON.stringify({
          to: form.get("to"),
          subject: form.get("subject"),
          body: form.get("body"),
        }),
      });
      formElement.reset();
      setDraft({ to: "", subject: "", body: "" });
      setActiveTemplate(null);
      setNotice("Email sent through your Google account.");
      load();
    } catch (reason) {
      setNotice(
        reason instanceof Error ? reason.message : "Could not send email",
      );
    } finally {
      setSending(false);
    }
  }

  async function syncMail() {
    setSyncing(true);
    setNotice("Queueing a Gmail check...");
    try {
      await api<AcceptedJob>("/api/v1/email/sync", { method: "POST" });
      setNotice(
        "Gmail check queued. New customer replies will appear here and in notifications when it finishes.",
      );
    } catch (reason) {
      setNotice(
        reason instanceof Error ? reason.message : "Could not sync Gmail",
      );
    } finally {
      setSyncing(false);
    }
  }

  async function createTemplate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await api("/api/v1/email/templates", {
      method: "POST",
      body: JSON.stringify({
        name: form.get("name"),
        subject: form.get("subject"),
        body: form.get("body"),
      }),
    });
    setTemplateOpen(false);
    load();
  }

  async function markRead(item: Notification) {
    await api(`/api/v1/notifications/${item.id}`, {
      method: "PATCH",
      body: "{}",
    });
    setNotifications((current) =>
      current.map((candidate) =>
        candidate.id === item.id
          ? { ...candidate, readAt: new Date().toISOString() }
          : candidate,
      ),
    );
  }

  function useTemplate(template: EmailTemplate) {
    const contact = contacts.find(
      (item) => item.email.toLowerCase() === draft.to.toLowerCase(),
    );
    const account = accounts.find((item) => item.id === contact?.accountId);
    setActiveTemplate(template);
    setDraft((current) => ({
      ...current,
      subject: mergeTemplate(template.subject, contact, account),
      body: mergeTemplate(template.body, contact, account),
    }));
    setNotice(
      contact
        ? `Template “${template.name}” is ready to review.`
        : `Template “${template.name}” is ready. Choose a known contact to fill its variables.`,
    );
  }

  function updateRecipient(to: string) {
    const contact = contacts.find(
      (item) => item.email.toLowerCase() === to.toLowerCase(),
    );
    const account = accounts.find((item) => item.id === contact?.accountId);
    setDraft((current) => ({
      ...current,
      to,
      ...(activeTemplate
        ? {
            subject: mergeTemplate(activeTemplate.subject, contact, account),
            body: mergeTemplate(activeTemplate.body, contact, account),
          }
        : {}),
    }));
  }

  function chooseVoiceContact(contactID: string) {
    setVoiceContactID(contactID);
    setPhone(contacts.find((contact) => contact.id === contactID)?.phone ?? "");
  }

  if (loading) return <LoadingState />;
  if (error) return <ErrorState message={error} retry={load} />;

  return (
    <Page
      eyebrow="Conversations"
      title="Communications"
      detail="Send intentional emails, notice customer replies, and jump into Google Voice without turning Kosmos into another inbox."
    >
      {!status?.connected && (
        <section className="tip-banner integration-banner">
          <span className="tip-icon">
            <Mail size={20} />
          </span>
          <span>
            <strong>Connect Google Workspace</strong>
            <small>
              Grant Gmail compose and metadata access plus read-only Tiller
              sheet access. Kosmos never stores message bodies from your inbox.
            </small>
          </span>
          <a
            className="banner-button"
            href={status?.connectUrl ?? "/auth/connect/workspace"}
          >
            Connect Google <ExternalLink size={15} />
          </a>
        </section>
      )}
      {status?.connected && (
        <div className="status-strip">
          <span className="security-dot" />
          <strong>{status.connection?.googleEmail}</strong> is connected{" "}
          <button className="text-button" onClick={syncMail} disabled={syncing}>
            <RefreshCw size={15} />{" "}
            {syncing ? "Queueing..." : "Check for replies"}
          </button>
        </div>
      )}
      {notice && (
        <p className="inline-notice" role="status">
          {notice}
        </p>
      )}
      <section className="split-grid communication-grid">
        <div className="panel compose-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Outbound</p>
              <h2>Send one good email</h2>
            </div>
            <button
              className="text-button"
              onClick={() => setTemplateOpen(true)}
            >
              <Plus size={15} /> Template
            </button>
          </div>
          <form className="compose-form" onSubmit={sendEmail}>
            <label>
              To
              <input
                name="to"
                type="email"
                list="known-contacts"
                required
                placeholder="customer@example.com"
                value={draft.to}
                onChange={(event) => updateRecipient(event.target.value)}
              />
            </label>
            <datalist id="known-contacts">
              {contacts
                .filter((contact) => contact.email)
                .map((contact) => (
                  <option value={contact.email} key={contact.id}>
                    {contact.name}
                  </option>
                ))}
            </datalist>
            <label>
              Subject
              <input
                name="subject"
                required
                maxLength={200}
                value={draft.subject}
                onChange={(event) => {
                  setActiveTemplate(null);
                  setDraft((current) => ({
                    ...current,
                    subject: event.target.value,
                  }));
                }}
              />
            </label>
            <label>
              Message
              <textarea
                name="body"
                required
                rows={9}
                value={draft.body}
                onChange={(event) => {
                  setActiveTemplate(null);
                  setDraft((current) => ({
                    ...current,
                    body: event.target.value,
                  }));
                }}
              />
            </label>
            {(draft.subject || draft.body) && (
              <section className="email-preview" aria-label="Email preview">
                <p className="eyebrow">Preview</p>
                <strong>{draft.subject || "(No subject)"}</strong>
                <p>{draft.body}</p>
              </section>
            )}
            <div className="form-actions">
              <button
                className="primary-button"
                disabled={!status?.connected || sending}
              >
                <Send size={16} /> {sending ? "Sending..." : "Send with Gmail"}
              </button>
            </div>
          </form>
          {!!templates.length && (
            <div className="template-list">
              <p className="eyebrow">Saved templates</p>
              {templates.map((template) => (
                <button
                  key={template.id}
                  className="record-row compact"
                  type="button"
                  onClick={() => useTemplate(template)}
                >
                  <strong>{template.name}</strong>
                  <small>{template.subject}</small>
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Inbound</p>
              <h2>Customer replies</h2>
            </div>
          </div>
          {messages.length ? (
            <div className="activity-list">
              {messages.map((message) => (
                <article className="activity-item" key={message.id}>
                  <span className="activity-icon">
                    <Mail size={16} />
                  </span>
                  <span>
                    <strong>{message.subject || "(No subject)"}</strong>
                    <small>{message.from}</small>
                    <time>{shortDate(message.receivedAt)}</time>
                  </span>
                </article>
              ))}
            </div>
          ) : (
            <EmptyState
              title="No customer replies yet"
              detail="When a known contact emails your connected Gmail account, the metadata appears here."
            />
          )}
        </div>
      </section>
      <section className="split-grid lower-grid">
        <div className="panel voice-panel">
          <p className="eyebrow">Google Voice</p>
          <h2>Call or text a contact</h2>
          <p className="muted-copy">
            Choose a saved person or enter any number. Kosmos opens your phone,
            messaging app, or Google Voice without pretending Voice has an
            automation API.
          </p>
          <div className="voice-fields">
            <label>
              Contact
              <select
                aria-label="Google Voice contact"
                value={voiceContactID}
                onChange={(event) => chooseVoiceContact(event.target.value)}
              >
                <option value="">Enter a number manually</option>
                {contacts
                  .filter((contact) => contact.phone)
                  .map((contact) => (
                    <option value={contact.id} key={contact.id}>
                      {contact.name} · {contact.phone}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              Phone number
              <input
                aria-label="Google Voice phone number"
                value={phone}
                onChange={(event) => {
                  setPhone(event.target.value);
                  setVoiceContactID("");
                }}
                inputMode="tel"
                placeholder="+1 555 123 4567"
              />
            </label>
          </div>
          <div className="button-row">
            {phone ? (
              <>
                <a className="secondary-button" href={`tel:${phone}`}>
                  Call
                </a>
                <a className="secondary-button" href={`sms:${phone}`}>
                  Text
                </a>
              </>
            ) : (
              <>
                <button className="secondary-button" disabled>
                  Call
                </button>
                <button className="secondary-button" disabled>
                  Text
                </button>
              </>
            )}
            <GoogleVoiceButton phone={phone} className="primary-button" />
          </div>
        </div>
        <div className="panel">
          <p className="eyebrow">Across Kosmos</p>
          <h2>Notifications</h2>
          {notifications.length ? (
            <div className="activity-list">
              {notifications.slice(0, 8).map((item) => (
                <article
                  className={`activity-item ${item.readAt ? "is-read" : ""}`}
                  key={item.id}
                >
                  <span className="activity-icon lavender">
                    <MessageSquareText size={16} />
                  </span>
                  <span>
                    <strong>{item.title}</strong>
                    <small>{item.summary}</small>
                    <time>{shortDate(item.createdAt)}</time>
                  </span>
                  {!item.readAt && (
                    <button
                      className="text-button"
                      onClick={() => markRead(item)}
                    >
                      Mark read
                    </button>
                  )}
                </article>
              ))}
            </div>
          ) : (
            <EmptyState
              title="You are all caught up"
              detail="Leads, email, transactions, and reminders will collect here."
            />
          )}
        </div>
      </section>
      {templateOpen && (
        <Modal
          eyebrow="Reusable message"
          title="New email template"
          onClose={() => setTemplateOpen(false)}
        >
          <form onSubmit={createTemplate}>
            <div
              className="template-variables"
              role="note"
              aria-label="Available template variables"
            >
              <strong>Available variables</strong>
              <span>
                <code>{"{{name}}"}</code> contact name
              </span>
              <span>
                <code>{"{{company}}"}</code> account name
              </span>
              <span>
                <code>{"{{domains}}"}</code> account domains
              </span>
            </div>
            <label>
              Name
              <input name="name" required autoFocus />
            </label>
            <label>
              Subject
              <input name="subject" required />
            </label>
            <label>
              Message
              <textarea name="body" rows={8} required />
            </label>
            <div className="form-actions">
              <button
                type="button"
                className="secondary-button"
                onClick={() => setTemplateOpen(false)}
              >
                Cancel
              </button>
              <button className="primary-button">Save template</button>
            </div>
          </form>
        </Modal>
      )}
    </Page>
  );
}
