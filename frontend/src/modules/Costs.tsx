import { FormEvent, useCallback, useEffect, useState } from "react";
import { Download, Plus, ReceiptText, Repeat2 } from "lucide-react";
import { api, Attachment, Cost, money, shortDate } from "../api";
import { Modal } from "../components/Modal";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";

export function Costs({
  embedded = false,
  initialItems,
}: {
  embedded?: boolean;
  initialItems?: Cost[];
}) {
  const [items, setItems] = useState<Cost[]>(initialItems ?? []);
  const [creating, setCreating] = useState(false);
  const [recurring, setRecurring] = useState(false);
  const [loading, setLoading] = useState(initialItems === undefined);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [receipts, setReceipts] = useState<Attachment[]>([]);
  const [pendingCostID, setPendingCostID] = useState("");

  const load = useCallback(() => {
    setLoading(true);
    api<{ costs: Cost[] }>("/api/v1/costs")
      .then((response) => setItems(response.costs))
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    if (initialItems !== undefined) {
      setItems(initialItems);
      setLoading(false);
      return;
    }
    load();
  }, [initialItems, load]);
  useEffect(() => {
    api<{ attachments: Attachment[] }>("/api/v1/attachments")
      .then((response) =>
        setReceipts(
          response.attachments.filter((item) => item.kind === "receipt"),
        ),
      )
      .catch(() => setReceipts([]));
  }, []);

  async function uploadReceipt(file: File, costID: string) {
    const data = new FormData();
    data.set("file", file);
    data.set("kind", "receipt");
    data.set("recordType", "cost");
    data.set("recordId", costID);
    const response = await fetch("/api/v1/attachments", {
      method: "POST",
      headers: { "X-Kosmos-CSRF": "1" },
      body: data,
    });
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as {
        error?: { message?: string };
      };
      throw new Error(
        body.error?.message ??
          "Cost saved, but the receipt could not be uploaded",
      );
    }
    const receipt = (await response.json()) as Attachment;
    setReceipts((current) => [receipt, ...current]);
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    const receiptFile = (
      event.currentTarget.elements.namedItem(
        "receipt",
      ) as HTMLInputElement | null
    )?.files?.[0];
    const form = new FormData(event.currentTarget);
    try {
      const created = pendingCostID
        ? items.find((item) => item.id === pendingCostID)!
        : await api<Cost>("/api/v1/costs", {
            method: "POST",
            body: JSON.stringify({
              vendor: form.get("vendor"),
              description: form.get("description"),
              amountCents: Math.round(Number(form.get("amount")) * 100),
              category: form.get("category"),
              incurredOn: form.get("incurredOn"),
              recurring,
              recurrence: recurring ? form.get("recurrence") : "",
              taxDeductible: form.get("taxDeductible") === "on",
              notes: form.get("notes"),
              renewalDate: form.get("renewalDate"),
              paymentMethod: form.get("paymentMethod"),
              reviewState: form.get("reviewState"),
            }),
          });
      if (!pendingCostID) {
        setItems((current) => [created, ...current]);
        setPendingCostID(created.id);
      }
      if (receiptFile) await uploadReceipt(receiptFile, created.id);
      setCreating(false);
      setRecurring(false);
      setPendingCostID("");
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save cost",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingState label="Loading business costs" />;
  if (error) return <ErrorState message={error} retry={load} />;

  const total = items.reduce((sum, item) => sum + item.amountCents, 0);
  const action = (
    <button className="primary-button" onClick={() => setCreating(true)}>
      <Plus size={17} /> Record a cost
    </button>
  );
  const content = items.length ? (
    <>
      <section className="cost-summary">
        <span>
          <small>Total recorded</small>
          <strong>{money(total)}</strong>
        </span>
        <span>
          <small>Recurring</small>
          <strong>{items.filter((item) => item.recurring).length}</strong>
        </span>
        <span>
          <small>Tax-deductible</small>
          <strong>{items.filter((item) => item.taxDeductible).length}</strong>
        </span>
      </section>
      <div className="record-table" role="table" aria-label="Business costs">
        {items.map((item) => {
          const files = receipts.filter(
            (receipt) => receipt.recordId === item.id,
          );
          return (
            <article className="cost-row" role="row" key={item.id}>
              <span className="cost-icon">
                {item.recurring ? (
                  <Repeat2 size={18} />
                ) : (
                  <ReceiptText size={18} />
                )}
              </span>
              <span className="record-main">
                <strong>{item.description}</strong>
                <small>
                  {[
                    item.vendor,
                    item.category,
                    shortDate(item.incurredOn + "T12:00:00Z"),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </small>
                {files.map((receipt) => (
                  <a
                    className="receipt-link"
                    href={receipt.downloadUrl}
                    key={receipt.id}
                  >
                    <Download size={13} /> {receipt.fileName}
                  </a>
                ))}
              </span>
              <span className="cost-flags">
                {item.taxDeductible && <small>Tax</small>}
                {item.recurring && <small>{item.recurrence}</small>}
              </span>
              <strong className="cost-amount">{money(item.amountCents)}</strong>
            </article>
          );
        })}
      </div>
    </>
  ) : (
    <EmptyState
      title="No costs recorded"
      detail="Start with a subscription or registration fee you pay every month."
      action={
        <button className="primary-button" onClick={() => setCreating(true)}>
          <Plus size={17} /> Record your first cost
        </button>
      }
    />
  );

  return (
    <>
      {embedded ? (
        <section className="panel operations-costs" id="costs">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Money</p>
              <h2>Business costs</h2>
              <p className="muted-copy">
                Track subscriptions, registrations, and every expense you will
                want at tax time.
              </p>
            </div>
            {action}
          </div>
          {content}
        </section>
      ) : (
        <Page
          eyebrow="Money"
          title="Business costs"
          detail="Track subscriptions, registrations, and every expense you will want at tax time."
          action={action}
        >
          {content}
        </Page>
      )}
      {creating && (
        <Modal
          eyebrow="Money"
          title="Record a business cost"
          onClose={() => {
            setCreating(false);
            setPendingCostID("");
          }}
        >
          <form onSubmit={create}>
            <div className="field-grid">
              <label>
                Description
                <input name="description" maxLength={200} required autoFocus />
              </label>
              <label>
                Vendor
                <input name="vendor" maxLength={160} />
              </label>
              <label>
                Amount
                <input
                  name="amount"
                  type="number"
                  min="0"
                  step="0.01"
                  inputMode="decimal"
                  required
                />
              </label>
              <label>
                Date
                <input
                  name="incurredOn"
                  type="date"
                  defaultValue={localDateValue()}
                  required
                />
              </label>
              <label>
                Category
                <input name="category" maxLength={100} placeholder="Software" />
              </label>
              <label>
                Payment method
                <input
                  name="paymentMethod"
                  maxLength={100}
                  placeholder="Business card"
                />
              </label>
              <label>
                Renewal date
                <input name="renewalDate" type="date" />
              </label>
              <label>
                Review state
                <select name="reviewState" defaultValue="ready">
                  <option value="ready">Ready</option>
                  <option value="review">Needs review</option>
                  <option value="complete">Complete</option>
                </select>
              </label>
              <label>
                Receipt
                <input
                  name="receipt"
                  type="file"
                  accept="image/jpeg,image/png,image/webp,application/pdf,text/plain"
                />
              </label>
              <label className="check-label">
                <input name="taxDeductible" type="checkbox" /> Tax-deductible
              </label>
              <label className="check-label">
                <input
                  name="recurring"
                  type="checkbox"
                  checked={recurring}
                  onChange={(event) => setRecurring(event.target.checked)}
                />{" "}
                Recurring cost
              </label>
              {recurring && (
                <label>
                  Repeats
                  <select name="recurrence" defaultValue="monthly">
                    <option value="monthly">Monthly</option>
                    <option value="quarterly">Quarterly</option>
                    <option value="yearly">Yearly</option>
                  </select>
                </label>
              )}
            </div>
            <label>
              Notes
              <textarea name="notes" rows={3} maxLength={1000} />
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
                  setPendingCostID("");
                }}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving
                  ? "Saving..."
                  : pendingCostID
                    ? "Retry receipt"
                    : "Save cost"}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

export function localDateValue(date = new Date()) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}
