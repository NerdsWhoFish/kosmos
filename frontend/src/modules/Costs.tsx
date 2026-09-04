import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  Download,
  Pencil,
  Plus,
  ReceiptText,
  Repeat2,
  Trash2,
} from "lucide-react";
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
  const [editing, setEditing] = useState<Cost | null>(null);
  const [deleting, setDeleting] = useState<Cost | null>(null);

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

  function openCreate() {
    setEditing(null);
    setRecurring(false);
    setPendingCostID("");
    setFormError("");
    setCreating(true);
  }

  function openEdit(item: Cost) {
    setEditing(item);
    setRecurring(item.recurring);
    setPendingCostID("");
    setFormError("");
    setCreating(true);
  }

  function closeForm() {
    setCreating(false);
    setEditing(null);
    setPendingCostID("");
  }

  async function save(event: FormEvent<HTMLFormElement>) {
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
      const saved = pendingCostID
        ? items.find((item) => item.id === pendingCostID)!
        : await api<Cost>(
            editing ? `/api/v1/costs/${editing.id}` : "/api/v1/costs",
            {
              method: editing ? "PATCH" : "POST",
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
            },
          );
      if (!pendingCostID) {
        setItems((current) =>
          editing
            ? current.map((item) => (item.id === saved.id ? saved : item))
            : [saved, ...current],
        );
        setPendingCostID(saved.id);
      }
      if (receiptFile) await uploadReceipt(receiptFile, saved.id);
      closeForm();
      setRecurring(false);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save cost",
      );
    } finally {
      setSaving(false);
    }
  }

  async function deleteCost() {
    if (!deleting) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/costs/${deleting.id}`, { method: "DELETE" });
      setItems((current) => current.filter((item) => item.id !== deleting.id));
      setReceipts((current) =>
        current.filter((receipt) => receipt.recordId !== deleting.id),
      );
      setDeleting(null);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not delete cost",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingState label="Loading business costs" />;
  if (error) return <ErrorState message={error} retry={load} />;

  const total = items.reduce((sum, item) => sum + item.amountCents, 0);
  const action = (
    <button className="primary-button" onClick={openCreate}>
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
              <span className="cost-actions">
                <button
                  className="record-action"
                  aria-label={`Edit ${item.description} cost`}
                  onClick={() => openEdit(item)}
                >
                  <Pencil size={15} /> Edit
                </button>
                <button
                  className="record-action danger-text"
                  aria-label={`Delete ${item.description} cost`}
                  onClick={() => setDeleting(item)}
                >
                  <Trash2 size={15} /> Delete
                </button>
              </span>
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
        <button className="primary-button" onClick={openCreate}>
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
          title={editing ? "Edit business cost" : "Record a business cost"}
          onClose={closeForm}
        >
          <form onSubmit={save}>
            <div className="field-grid">
              <label>
                Description
                <input
                  name="description"
                  maxLength={200}
                  required
                  autoFocus
                  defaultValue={editing?.description}
                />
              </label>
              <label>
                Vendor
                <input
                  name="vendor"
                  maxLength={160}
                  defaultValue={editing?.vendor}
                />
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
                  defaultValue={
                    editing ? (editing.amountCents / 100).toFixed(2) : undefined
                  }
                />
              </label>
              <label>
                Date
                <input
                  name="incurredOn"
                  type="date"
                  defaultValue={editing?.incurredOn || localDateValue()}
                  required
                />
              </label>
              <label>
                Category
                <input
                  name="category"
                  maxLength={100}
                  placeholder="Software"
                  defaultValue={editing?.category}
                />
              </label>
              <label>
                Payment method
                <input
                  name="paymentMethod"
                  maxLength={100}
                  placeholder="Business card"
                  defaultValue={editing?.paymentMethod}
                />
              </label>
              <label>
                Renewal date
                <input
                  name="renewalDate"
                  type="date"
                  defaultValue={editing?.renewalDate}
                />
              </label>
              <label>
                Review state
                <select
                  name="reviewState"
                  defaultValue={editing?.reviewState || "ready"}
                >
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
                <input
                  name="taxDeductible"
                  type="checkbox"
                  defaultChecked={editing?.taxDeductible}
                />{" "}
                Tax-deductible
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
                  <select
                    name="recurrence"
                    defaultValue={editing?.recurrence || "monthly"}
                  >
                    <option value="monthly">Monthly</option>
                    <option value="quarterly">Quarterly</option>
                    <option value="yearly">Yearly</option>
                  </select>
                </label>
              )}
            </div>
            <label>
              Notes
              <textarea
                name="notes"
                rows={3}
                maxLength={1000}
                defaultValue={editing?.notes}
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
                onClick={closeForm}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving
                  ? "Saving..."
                  : pendingCostID
                    ? "Retry receipt"
                    : editing
                      ? "Save changes"
                      : "Save cost"}
              </button>
            </div>
          </form>
        </Modal>
      )}
      {deleting && (
        <Modal
          eyebrow="Money"
          title="Delete this business cost?"
          onClose={() => setDeleting(null)}
        >
          <p className="muted-copy">
            {deleting.description} and its linked receipt files will be
            permanently deleted.
          </p>
          {formError && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
          <div className="form-actions">
            <button
              className="secondary-button"
              onClick={() => setDeleting(null)}
            >
              Keep cost
            </button>
            <button
              className="danger-button"
              onClick={deleteCost}
              disabled={saving}
            >
              <Trash2 size={16} /> {saving ? "Deleting..." : "Delete cost"}
            </button>
          </div>
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
