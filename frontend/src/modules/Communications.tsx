import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  ExternalLink,
  Mail,
  MessageSquareText,
  Pencil,
  Plus,
  RefreshCw,
  Send,
  Trash2,
} from "lucide-react";
import {
  AcceptedJob,
  Account,
  api,
  Contact,
  EmailTemplate,
  EmailTemplateInput,
  GoogleStatus,
  MailMessage,
  Notification,
  shortDate,
} from "../api";
import { GoogleVoiceButton } from "../components/GoogleVoiceButton";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { WorkflowPage } from "../components/WorkflowPage";
import { ResourceRoute } from "../routing";

export function mergeTemplate(
  value: string,
  contact?: Contact,
  account?: Account,
  answers: Record<string, string> = {},
) {
  const values: Record<string, string> = {
    name: contact?.name || "",
    company: account?.name || "",
    domains:
      account?.websites
        ?.map((website) => website.domain || website.url)
        .filter(Boolean)
        .join(", ") || "",
    ...answers,
  };
  return value.replace(/{{\s*([a-z][a-z0-9_]*)\s*}}/g, (token, key: string) =>
    values[key]?.trim() ? values[key] : token,
  );
}

export function hasTemplateVariables(value: string) {
  return /{{\s*[a-z][a-z0-9_]*\s*}}/.test(value);
}

export function Communications({
  route,
  navigate,
}: {
  route: ResourceRoute;
  navigate: (path: string) => void;
}) {
  const [status, setStatus] = useState<GoogleStatus | null>(null);
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [templateInputs, setTemplateInputs] = useState<EmailTemplateInput[]>(
    [],
  );
  const [templateAnswers, setTemplateAnswers] = useState<
    Record<string, string>
  >({});
  const [templateSaving, setTemplateSaving] = useState(false);
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

  const editingTemplate =
    route.action === "edit"
      ? (templates.find((item) => item.id === route.id) ?? null)
      : null;
  const deletingTemplate =
    route.action === "delete"
      ? (templates.find((item) => item.id === route.id) ?? null)
      : null;
  useEffect(() => {
    if (route.action === "new") setTemplateInputs([]);
    if (route.action === "edit" && editingTemplate)
      setTemplateInputs(
        (editingTemplate.inputs ?? []).map((input) => ({ ...input })),
      );
    setNotice("");
  }, [route.action, route.id, editingTemplate?.id]);

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
      setTemplateAnswers({});
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

  async function saveTemplate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setTemplateSaving(true);
    setNotice("");
    try {
      await api(
        editingTemplate
          ? `/api/v1/email/templates/${editingTemplate.id}`
          : "/api/v1/email/templates",
        {
          method: editingTemplate ? "PATCH" : "POST",
          body: JSON.stringify({
            name: form.get("name"),
            subject: form.get("subject"),
            body: form.get("body"),
            inputs: templateInputs,
          }),
        },
      );
      setNotice(
        editingTemplate ? "Email template updated." : "Email template saved.",
      );
      await load();
      navigate("/communications");
    } catch (reason) {
      setNotice(
        reason instanceof Error ? reason.message : "Could not save template",
      );
    } finally {
      setTemplateSaving(false);
    }
  }

  async function deleteTemplate() {
    if (!deletingTemplate) return;
    setTemplateSaving(true);
    setNotice("");
    try {
      await api(`/api/v1/email/templates/${deletingTemplate.id}`, {
        method: "DELETE",
      });
      setTemplates((current) =>
        current.filter((item) => item.id !== deletingTemplate.id),
      );
      if (activeTemplate?.id === deletingTemplate.id) setActiveTemplate(null);
      setNotice("Email template deleted.");
      navigate("/communications");
    } catch (reason) {
      setNotice(
        reason instanceof Error ? reason.message : "Could not delete template",
      );
    } finally {
      setTemplateSaving(false);
    }
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
    const answers = Object.fromEntries(
      (template.inputs ?? []).map((input) => [input.key, input.defaultValue]),
    );
    setActiveTemplate(template);
    setTemplateAnswers(answers);
    setDraft((current) => ({
      ...current,
      subject: mergeTemplate(template.subject, contact, account, answers),
      body: mergeTemplate(template.body, contact, account, answers),
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
            subject: mergeTemplate(
              activeTemplate.subject,
              contact,
              account,
              templateAnswers,
            ),
            body: mergeTemplate(
              activeTemplate.body,
              contact,
              account,
              templateAnswers,
            ),
          }
        : {}),
    }));
  }

  function updateTemplateAnswer(key: string, value: string) {
    if (!activeTemplate) return;
    const answers = { ...templateAnswers, [key]: value };
    const contact = contacts.find(
      (item) => item.email.toLowerCase() === draft.to.toLowerCase(),
    );
    const account = accounts.find((item) => item.id === contact?.accountId);
    setTemplateAnswers(answers);
    setDraft((current) => ({
      ...current,
      subject: mergeTemplate(activeTemplate.subject, contact, account, answers),
      body: mergeTemplate(activeTemplate.body, contact, account, answers),
    }));
  }

  function chooseVoiceContact(contactID: string) {
    setVoiceContactID(contactID);
    setPhone(contacts.find((contact) => contact.id === contactID)?.phone ?? "");
  }

  if (loading) return <LoadingState />;
  if (error) return <ErrorState message={error} retry={load} />;

  if (route.action === "new" || route.action === "edit") {
    if (route.action === "edit" && !editingTemplate)
      return <ErrorState message="That email template could not be found." />;
    return (
      <WorkflowPage
        eyebrow="Reusable message"
        title={editingTemplate ? "Edit email template" : "New email template"}
        detail="Add questions for details that change each time. Senders must answer every question without a default."
        backLabel="Back to communications"
        onBack={() => navigate("/communications")}
      >
        <form onSubmit={saveTemplate}>
          <div
            className="template-variables"
            role="note"
            aria-label="Available template variables"
          >
            <strong>Built-in variables</strong>
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
            <input
              name="name"
              required
              autoFocus
              defaultValue={editingTemplate?.name}
            />
          </label>
          <label>
            Subject
            <input
              name="subject"
              required
              defaultValue={editingTemplate?.subject}
            />
          </label>
          <label>
            Message
            <textarea
              name="body"
              rows={12}
              required
              defaultValue={editingTemplate?.body}
            />
          </label>
          <fieldset className="form-section template-inputs">
            <legend>Questions for the sender</legend>
            <p className="field-help">
              Each answer replaces its matching variable in the subject or
              message.
            </p>
            {templateInputs.map((input, index) => (
              <div className="template-input-card" key={index}>
                <div className="field-grid">
                  <label>
                    Variable key
                    <input
                      aria-label={`Question ${index + 1} variable key`}
                      value={input.key}
                      pattern="[a-z][a-z0-9_]*"
                      placeholder="renewal_amount"
                      required
                      onChange={(event) =>
                        setTemplateInputs((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index
                              ? {
                                  ...item,
                                  key: event.target.value
                                    .toLowerCase()
                                    .replace(/[^a-z0-9_]/g, "_"),
                                }
                              : item,
                          ),
                        )
                      }
                    />
                  </label>
                  <label>
                    Question
                    <input
                      aria-label={`Question ${index + 1} label`}
                      value={input.label}
                      placeholder="How much is their renewal?"
                      required
                      onChange={(event) =>
                        setTemplateInputs((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, label: event.target.value }
                              : item,
                          ),
                        )
                      }
                    />
                  </label>
                  <label>
                    Default answer <small>(optional)</small>
                    <input
                      aria-label={`Question ${index + 1} default answer`}
                      value={input.defaultValue}
                      placeholder="$0.00"
                      onChange={(event) =>
                        setTemplateInputs((current) =>
                          current.map((item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, defaultValue: event.target.value }
                              : item,
                          ),
                        )
                      }
                    />
                  </label>
                </div>
                <div className="template-token-row">
                  <code>{`{{${input.key || "variable"}}}`}</code>
                  <button
                    type="button"
                    className="text-button danger-text"
                    onClick={() =>
                      setTemplateInputs((current) =>
                        current.filter((_, itemIndex) => itemIndex !== index),
                      )
                    }
                  >
                    <Trash2 size={15} /> Remove question
                  </button>
                </div>
              </div>
            ))}
            <button
              type="button"
              className="secondary-button"
              onClick={() =>
                setTemplateInputs((current) => [
                  ...current,
                  { key: "", label: "", defaultValue: "" },
                ])
              }
            >
              <Plus size={15} /> Add a question
            </button>
          </fieldset>
          {notice && (
            <p className="form-error" role="alert">
              {notice}
            </p>
          )}
          <div className="form-actions">
            <button
              type="button"
              className="secondary-button"
              onClick={() => navigate("/communications")}
            >
              Cancel
            </button>
            <button className="primary-button" disabled={templateSaving}>
              {templateSaving
                ? "Saving..."
                : editingTemplate
                  ? "Save changes"
                  : "Save template"}
            </button>
          </div>
        </form>
      </WorkflowPage>
    );
  }

  if (route.action === "delete") {
    if (!deletingTemplate)
      return <ErrorState message="That email template could not be found." />;
    return (
      <WorkflowPage
        eyebrow="Reusable message"
        title="Delete this email template?"
        detail={`${deletingTemplate.name} will be removed for everyone in the organization.`}
        backLabel="Keep template"
        onBack={() => navigate("/communications")}
        tone="danger"
      >
        {notice && (
          <p className="form-error" role="alert">
            {notice}
          </p>
        )}
        <div className="form-actions">
          <button
            className="secondary-button"
            onClick={() => navigate("/communications")}
          >
            Keep template
          </button>
          <button
            className="danger-button"
            onClick={deleteTemplate}
            disabled={templateSaving}
          >
            <Trash2 size={16} />{" "}
            {templateSaving ? "Deleting..." : "Delete template"}
          </button>
        </div>
      </WorkflowPage>
    );
  }

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
              onClick={() => navigate("/communications/templates/new")}
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
                  setDraft((current) => ({
                    ...current,
                    body: event.target.value,
                  }));
                }}
              />
            </label>
            {!!activeTemplate?.inputs?.length && (
              <fieldset className="form-section template-questionnaire">
                <legend>Fill in this template</legend>
                <p className="field-help">
                  These answers are required before the email can be sent.
                </p>
                {activeTemplate.inputs.map((input) => (
                  <label key={input.key}>
                    {input.label}
                    <input
                      required
                      value={templateAnswers[input.key] ?? ""}
                      onChange={(event) =>
                        updateTemplateAnswer(input.key, event.target.value)
                      }
                    />
                  </label>
                ))}
              </fieldset>
            )}
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
                disabled={
                  !status?.connected ||
                  sending ||
                  (activeTemplate?.inputs ?? []).some(
                    (input) => !(templateAnswers[input.key] ?? "").trim(),
                  )
                }
              >
                <Send size={16} /> {sending ? "Sending..." : "Send with Gmail"}
              </button>
            </div>
          </form>
          {!!templates.length && (
            <div className="template-list">
              <p className="eyebrow">Saved templates</p>
              {templates.map((template) => (
                <article
                  className="record-row compact template-row"
                  key={template.id}
                >
                  <button
                    className="template-main"
                    type="button"
                    onClick={() => useTemplate(template)}
                  >
                    <span>
                      <strong>{template.name}</strong>
                      <small>{template.subject}</small>
                    </span>
                  </button>
                  <span className="template-actions">
                    <button
                      className="record-action"
                      type="button"
                      aria-label={`Edit ${template.name} template`}
                      onClick={() =>
                        navigate(
                          `/communications/templates/${template.id}/edit`,
                        )
                      }
                    >
                      <Pencil size={15} /> Edit
                    </button>
                    <button
                      className="record-action danger-text"
                      type="button"
                      aria-label={`Delete ${template.name} template`}
                      onClick={() =>
                        navigate(
                          `/communications/templates/${template.id}/delete`,
                        )
                      }
                    >
                      <Trash2 size={15} /> Delete
                    </button>
                  </span>
                </article>
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
            <GoogleVoiceButton
              phone={phone}
              contactId={voiceContactID || undefined}
              className="primary-button"
            />
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
                  <span className="notification-copy">
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
    </Page>
  );
}
