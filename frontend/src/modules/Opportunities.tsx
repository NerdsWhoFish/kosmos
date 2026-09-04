import {
  DragEvent,
  FormEvent,
  KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { CircleDollarSign, Plus, Trash2 } from "lucide-react";
import {
  Account,
  api,
  Contact,
  money,
  Opportunity,
  PipelineStage,
  shortDate,
} from "../api";
import { Modal } from "../components/Modal";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";

type OpportunityView = "pipeline" | "won" | "lost";

export function Opportunities({
  initialView,
  navigate,
}: {
  initialView: OpportunityView;
  navigate: (path: string) => void;
}) {
  const [items, setItems] = useState<Opportunity[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [stages, setStages] = useState<PipelineStage[]>([]);
  const [view, setView] = useState<OpportunityView>(initialView);
  const [creating, setCreating] = useState(false);
  const [newAccountID, setNewAccountID] = useState("");
  const [draggedID, setDraggedID] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<Opportunity | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError("");
    Promise.all([
      api<{ opportunities: Opportunity[] }>("/api/v1/opportunities"),
      api<{ contacts: Contact[] }>("/api/v1/contacts"),
      api<{ accounts: Account[] }>("/api/v1/accounts"),
      api<{ stages: PipelineStage[] }>("/api/v1/pipeline-stages"),
    ])
      .then(([opportunities, people, businesses, pipeline]) => {
        setItems(opportunities.opportunities);
        setContacts(people.contacts);
        setAccounts(businesses.accounts);
        setStages(pipeline.stages);
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);
  useEffect(load, [load]);
  useEffect(() => setView(initialView), [initialView]);

  const openStages = useMemo(
    () => stages.filter((stage) => !stage.closed),
    [stages],
  );
  const wonStage =
    stages.find((stage) => stage.closed && stage.won)?.id ?? "won";
  const lostStage =
    stages.find((stage) => stage.closed && !stage.won)?.id ?? "lost";

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    const form = new FormData(event.currentTarget);
    try {
      const amount = Math.round(Number(form.get("amount")) * 100);
      const created = await api<Opportunity>("/api/v1/opportunities", {
        method: "POST",
        body: JSON.stringify({
          name: form.get("name"),
          accountId: form.get("accountId"),
          contactId: form.get("contactId"),
          amountCents: amount,
          stage: form.get("stage"),
          nextStep: form.get("nextStep"),
          closeDate: form.get("closeDate"),
        }),
      });
      setItems((current) => [created, ...current]);
      setCreating(false);
      setNewAccountID("");
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save opportunity",
      );
    } finally {
      setSaving(false);
    }
  }

  async function move(item: Opportunity, stage: string) {
    try {
      const updated = await api<Opportunity>(
        `/api/v1/opportunities/${item.id}`,
        { method: "PATCH", body: JSON.stringify({ stage }) },
      );
      setItems((current) =>
        current.map((candidate) =>
          candidate.id === updated.id ? updated : candidate,
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not move opportunity",
      );
    }
  }

  async function deleteOpportunity() {
    if (!deleting) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/opportunities/${deleting.id}`, { method: "DELETE" });
      setItems((current) => current.filter((item) => item.id !== deleting.id));
      setDeleting(null);
    } catch (reason) {
      setFormError(
        reason instanceof Error
          ? reason.message
          : "Could not delete opportunity",
      );
    } finally {
      setSaving(false);
    }
  }

  function drop(event: DragEvent, stage: string) {
    event.preventDefault();
    const id = event.dataTransfer.getData("text/plain") || draggedID;
    const item = items.find((candidate) => candidate.id === id);
    setDraggedID("");
    if (item && item.stage !== stage) void move(item, stage);
  }

  function openAccount(item: Opportunity) {
    const accountID =
      item.accountId ||
      contacts.find((contact) => contact.id === item.contactId)?.accountId;
    if (accountID) navigate(`/accounts/${accountID}`);
  }

  function keyOpen(event: KeyboardEvent, item: Opportunity) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openAccount(item);
    }
  }

  if (loading) return <LoadingState label="Loading your pipeline" />;
  if (error) return <ErrorState message={error} retry={load} />;

  const closedItems = [...items]
    .filter((item) =>
      view === "won" ? item.stage === wonStage : item.stage === lostStage,
    )
    .sort(
      (a, b) =>
        new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
    );
  const allStageChoices = stages.length
    ? stages
    : ([
        { id: "new", name: "New" },
        { id: "won", name: "Won" },
        { id: "lost", name: "Lost" },
      ] as PipelineStage[]);

  return (
    <>
      <Page
        eyebrow="Pipeline"
        title="Opportunities"
        detail="Know what is moving, what is stuck, and what each win is worth."
        action={
          <button className="primary-button" onClick={() => setCreating(true)}>
            <Plus size={17} /> Add opportunity
          </button>
        }
      >
        <div
          className="opportunity-tabs"
          role="tablist"
          aria-label="Opportunity views"
        >
          {(["pipeline", "won", "lost"] as OpportunityView[]).map((choice) => (
            <button
              role="tab"
              aria-selected={view === choice}
              className={view === choice ? "active" : ""}
              key={choice}
              onClick={() => setView(choice)}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) =>
                drop(
                  event,
                  choice === "won"
                    ? wonStage
                    : choice === "lost"
                      ? lostStage
                      : (openStages[0]?.id ?? "new"),
                )
              }
            >
              {choice}
            </button>
          ))}
        </div>
        {view === "pipeline" ? (
          openStages.length ? (
            <div className="pipeline-board">
              {openStages.map((stage) => (
                <section
                  className={`pipeline-column ${stage.id}`}
                  role="region"
                  aria-label={`${stage.name} stage`}
                  key={stage.id}
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={(event) => drop(event, stage.id)}
                >
                  <header>
                    <span>{stage.name}</span>
                    <strong>
                      {items.filter((item) => item.stage === stage.id).length}
                    </strong>
                  </header>
                  <div>
                    {items
                      .filter((item) => item.stage === stage.id)
                      .map((item) => (
                        <OpportunityCard
                          key={item.id}
                          item={item}
                          accounts={accounts}
                          contacts={contacts}
                          stages={allStageChoices}
                          onMove={move}
                          onOpen={openAccount}
                          onKeyOpen={keyOpen}
                          onDelete={setDeleting}
                          onDragStart={(event) => {
                            setDraggedID(item.id);
                            event.dataTransfer.effectAllowed = "move";
                            event.dataTransfer.setData("text/plain", item.id);
                          }}
                        />
                      ))}
                  </div>
                </section>
              ))}
            </div>
          ) : (
            <EmptyState
              title="No open pipeline stages"
              detail="Add an open stage in Settings."
            />
          )
        ) : closedItems.length ? (
          <div className="closed-opportunity-list">
            {closedItems.map((item) => (
              <OpportunityCard
                key={item.id}
                item={item}
                accounts={accounts}
                contacts={contacts}
                stages={allStageChoices}
                onMove={move}
                onOpen={openAccount}
                onKeyOpen={keyOpen}
                onDelete={setDeleting}
                onDragStart={(event) => {
                  setDraggedID(item.id);
                  event.dataTransfer.effectAllowed = "move";
                  event.dataTransfer.setData("text/plain", item.id);
                }}
              />
            ))}
          </div>
        ) : (
          <EmptyState
            title={`No ${view} opportunities`}
            detail={`${view === "won" ? "Closed wins" : "Closed losses"} will collect here newest first.`}
          />
        )}
      </Page>
      {creating && (
        <Modal
          eyebrow="Pipeline"
          title="Add an opportunity"
          onClose={() => setCreating(false)}
        >
          <form onSubmit={create}>
            <label>
              Opportunity name
              <input name="name" maxLength={160} required autoFocus />
            </label>
            <div className="field-grid">
              <label>
                Account
                <select
                  name="accountId"
                  value={newAccountID}
                  onChange={(event) => setNewAccountID(event.target.value)}
                  required
                >
                  <option value="">Choose an account</option>
                  {accounts.map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Contact
                <select name="contactId" defaultValue="">
                  <option value="">No contact needed</option>
                  {contacts
                    .filter((contact) => contact.accountId === newAccountID)
                    .map((contact) => (
                      <option key={contact.id} value={contact.id}>
                        {contact.name}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                Value
                <input
                  name="amount"
                  type="number"
                  inputMode="decimal"
                  min="0"
                  step="0.01"
                  defaultValue="0"
                  required
                />
              </label>
              <label>
                Stage
                <select name="stage" defaultValue={openStages[0]?.id ?? "new"}>
                  {openStages.map((stage) => (
                    <option key={stage.id} value={stage.id}>
                      {stage.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Target close
                <input name="closeDate" type="date" />
              </label>
            </div>
            <label>
              Next step
              <input
                name="nextStep"
                maxLength={240}
                placeholder="Send the proposal"
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
                onClick={() => setCreating(false)}
              >
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                {saving ? "Saving..." : "Save opportunity"}
              </button>
            </div>
          </form>
        </Modal>
      )}
      {deleting && (
        <Modal
          eyebrow="Pipeline"
          title="Delete this opportunity?"
          onClose={() => setDeleting(null)}
        >
          <p className="muted-copy">
            {deleting.name} will be permanently removed from the pipeline.
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
              Keep opportunity
            </button>
            <button
              className="danger-button"
              onClick={deleteOpportunity}
              disabled={saving}
            >
              <Trash2 size={16} />{" "}
              {saving ? "Deleting..." : "Delete opportunity"}
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}

function OpportunityCard({
  item,
  accounts,
  contacts,
  stages,
  onMove,
  onOpen,
  onKeyOpen,
  onDelete,
  onDragStart,
}: {
  item: Opportunity;
  accounts: Account[];
  contacts: Contact[];
  stages: PipelineStage[];
  onMove: (item: Opportunity, stage: string) => void;
  onOpen: (item: Opportunity) => void;
  onKeyOpen: (event: KeyboardEvent, item: Opportunity) => void;
  onDelete: (item: Opportunity) => void;
  onDragStart: (event: DragEvent<HTMLElement>) => void;
}) {
  return (
    <article
      className="opportunity-card"
      draggable
      onDragStart={onDragStart}
      role="link"
      tabIndex={0}
      aria-label={`Open ${item.name} account`}
      onClick={() => onOpen(item)}
      onKeyDown={(event) => onKeyOpen(event, item)}
    >
      <span className="opportunity-value">
        <CircleDollarSign size={15} />
        {money(item.amountCents)}
      </span>
      <h3>{item.name}</h3>
      <p>{accountName(accounts, item.accountId)}</p>
      {item.contactId && (
        <small>Contact: {contactName(contacts, item.contactId)}</small>
      )}
      {item.nextStep && <small>Next: {item.nextStep}</small>}
      {item.closeDate && (
        <time>Close {shortDate(item.closeDate + "T12:00:00Z")}</time>
      )}
      <label
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
      >
        Move to
        <select
          aria-label={`Stage for ${item.name}`}
          value={item.stage}
          onChange={(event) => onMove(item, event.target.value)}
        >
          {stages.map((choice) => (
            <option key={choice.id} value={choice.id}>
              {choice.name}
            </option>
          ))}
        </select>
      </label>
      <button
        className="opportunity-delete"
        aria-label={`Delete ${item.name}`}
        onClick={(event) => {
          event.stopPropagation();
          onDelete(item);
        }}
      >
        <Trash2 size={14} /> Delete
      </button>
    </article>
  );
}

function contactName(contacts: Contact[], id: string) {
  return (
    contacts.find((contact) => contact.id === id)?.name || "Unknown contact"
  );
}

function accountName(accounts: Account[], id: string) {
  return (
    accounts.find((account) => account.id === id)?.name || "Unknown account"
  );
}
