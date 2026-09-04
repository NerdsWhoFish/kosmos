import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, BadgeCheck, ScanLine } from "lucide-react";
import { Account, Activity, api, Contact } from "../api";
import { ContactSourcePicker } from "../components/ContactSourcePicker";
import { ErrorState, LoadingState } from "../components/States";

type AccountCreation = { account: Account; contact?: Contact };

export function LeadIntake({ navigate }: { navigate: (path: string) => void }) {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState<Contact | null>(null);
  const [formKey, setFormKey] = useState(0);

  useEffect(() => {
    api<{ accounts: Account[] }>("/api/v1/accounts")
      .then((response) => setAccounts(response.accounts))
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    const business = String(form.get("business") ?? "").trim();
    const contactInput = {
      name: form.get("name"),
      email: form.get("email"),
      phone: form.get("phone"),
      source: form.get("source"),
    };
    try {
      const existing = accounts.find(
        (account) => account.name.toLowerCase() === business.toLowerCase(),
      );
      let contact: Contact;
      if (existing) {
        contact = await api<Contact>("/api/v1/contacts", {
          method: "POST",
          body: JSON.stringify({ ...contactInput, accountId: existing.id }),
        });
      } else if (business) {
        const created = await api<AccountCreation>("/api/v1/accounts", {
          method: "POST",
          body: JSON.stringify({
            name: business,
            status: "prospect",
            primaryContact: contactInput,
          }),
        });
        if (!created.contact)
          throw new Error("The lead contact was not created");
        contact = created.contact;
        setAccounts((current) => [created.account, ...current]);
      } else {
        contact = await api<Contact>("/api/v1/contacts", {
          method: "POST",
          body: JSON.stringify(contactInput),
        });
      }

      const notes = String(form.get("notes") ?? "").trim();
      if (notes) {
        await api<Activity>("/api/v1/activities", {
          method: "POST",
          body: JSON.stringify({
            contactId: contact.id,
            kind: "note",
            body: notes,
          }),
        });
      }
      setSaved(contact);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not save lead",
      );
    } finally {
      setSaving(false);
    }
  }

  function addAnother() {
    setSaved(null);
    setFormKey((current) => current + 1);
  }

  if (loading) return <LoadingState label="Preparing quick lead entry" />;
  if (error && !saving && !saved && !accounts.length)
    return (
      <ErrorState message={error} retry={() => window.location.reload()} />
    );

  return (
    <section className="lead-intake">
      <header className="lead-intake-heading">
        <span className="lead-intake-mark" aria-hidden="true">
          <ScanLine />
        </span>
        <div>
          <p className="eyebrow">Event mode</p>
          <h1>Capture the conversation.</h1>
          <p>Name first. Everything else can wait until the booth is quiet.</p>
        </div>
      </header>
      {saved ? (
        <div className="lead-success" role="status">
          <BadgeCheck size={42} />
          <p className="eyebrow">Lead saved</p>
          <h2>{saved.name} is in Kosmos.</h2>
          <div className="lead-success-actions">
            <button className="primary-button" onClick={addAnother} autoFocus>
              Add another lead
            </button>
            <button
              className="secondary-button"
              onClick={() => navigate(`/contacts/${saved.id}`)}
            >
              Open contact <ArrowRight size={16} />
            </button>
          </div>
        </div>
      ) : (
        <form className="lead-intake-form" onSubmit={submit} key={formKey}>
          <label className="lead-name">
            Their name
            <input
              name="name"
              maxLength={160}
              autoComplete="name"
              required
              autoFocus
              placeholder="Jane Smith"
            />
          </label>
          <div className="lead-field-grid">
            <label>
              Phone
              <input
                name="phone"
                type="tel"
                inputMode="tel"
                autoComplete="tel"
                placeholder="(555) 123-4567"
              />
            </label>
            <label>
              Email
              <input
                name="email"
                type="email"
                inputMode="email"
                autoComplete="email"
                placeholder="jane@example.com"
              />
            </label>
          </div>
          <label>
            Business
            <input
              name="business"
              list="lead-accounts"
              maxLength={160}
              autoComplete="organization"
              placeholder="Existing or new business"
            />
          </label>
          <datalist id="lead-accounts">
            {accounts.map((account) => (
              <option value={account.name} key={account.id} />
            ))}
          </datalist>
          <ContactSourcePicker defaultValue="Event" />
          <label>
            Quick note <span className="optional-label">optional</span>
            <textarea
              name="notes"
              rows={3}
              maxLength={4000}
              placeholder="What did they need?"
            />
          </label>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <button className="primary-button lead-save" disabled={saving}>
            {saving ? "Saving lead..." : "Save lead"} <ArrowRight size={18} />
          </button>
        </form>
      )}
    </section>
  );
}
