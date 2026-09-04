import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import CodeMirror from "@uiw/react-codemirror";
import { markdown } from "@codemirror/lang-markdown";
import { EditorView } from "@codemirror/view";
import {
  Download,
  Edit3,
  FilePlus2,
  History,
  Paperclip,
  Save,
  Trash2,
  UploadCloud,
  X,
} from "lucide-react";
import ReactMarkdown from "react-markdown";
import {
  Account,
  api,
  Attachment,
  Contact,
  Cost,
  Document,
  DocumentRevision,
  Opportunity,
  RecordLink,
  shortDate,
} from "../api";
import { Modal } from "../components/Modal";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";

type LinkOption = RecordLink & { label: string };
type Draft = { title: string; body: string; links: RecordLink[] };

const editorTheme = EditorView.theme({
  "&": { backgroundColor: "var(--theme-surface)", color: "var(--theme-text)" },
  ".cm-content": {
    caretColor: "var(--theme-accent)",
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
  },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--theme-accent)" },
  ".cm-gutters": {
    backgroundColor: "var(--theme-surface-raised)",
    color: "var(--theme-muted)",
    border: "0",
  },
  ".cm-activeLine, .cm-activeLineGutter": {
    backgroundColor: "color-mix(in srgb, var(--theme-accent) 10%, transparent)",
  },
  ".cm-selectionBackground, ::selection": {
    backgroundColor:
      "color-mix(in srgb, var(--theme-accent) 32%, transparent) !important",
  },
});

export function Documents({ initialID = "" }: { initialID?: string }) {
  const [items, setItems] = useState<Document[]>([]);
  const [selectedID, setSelectedID] = useState(initialID);
  const [editorMode, setEditorMode] = useState<"create" | "edit" | null>(null);
  const [draft, setDraft] = useState<Draft>({ title: "", body: "", links: [] });
  const [deleting, setDeleting] = useState<Document | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [revisions, setRevisions] = useState<DocumentRevision[]>([]);
  const [linkOptions, setLinkOptions] = useState<LinkOption[]>([]);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const attachmentVersion = useRef(0);
  const selected = items.find((item) => item.id === selectedID);

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      api<{ documents: Document[] }>("/api/v1/documents"),
      api<{ accounts: Account[] }>("/api/v1/accounts"),
      api<{ contacts: Contact[] }>("/api/v1/contacts"),
      api<{ opportunities: Opportunity[] }>("/api/v1/opportunities"),
      api<{ costs: Cost[] }>("/api/v1/costs"),
    ])
      .then(([documents, accounts, contacts, opportunities, costs]) => {
        setItems(documents.documents);
        setSelectedID((current) => current || documents.documents[0]?.id || "");
        setLinkOptions([
          ...accounts.accounts.map((item) => ({
            type: "account" as const,
            id: item.id,
            label: item.name,
          })),
          ...contacts.contacts.map((item) => ({
            type: "contact" as const,
            id: item.id,
            label: item.name,
          })),
          ...opportunities.opportunities.map((item) => ({
            type: "opportunity" as const,
            id: item.id,
            label: item.name,
          })),
          ...costs.costs.map((item) => ({
            type: "cost" as const,
            id: item.id,
            label: item.description,
          })),
          ...documents.documents.map((item) => ({
            type: "document" as const,
            id: item.id,
            label: item.title,
          })),
        ]);
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(load, [load]);
  useEffect(() => {
    if (initialID) setSelectedID(initialID);
  }, [initialID]);
  useEffect(() => {
    const requestVersion = ++attachmentVersion.current;
    if (!selectedID) {
      setAttachments([]);
      setRevisions([]);
      return;
    }
    Promise.all([
      api<{ revisions: DocumentRevision[] }>(
        `/api/v1/documents/${selectedID}/revisions`,
      ),
      api<{ attachments: Attachment[] }>(
        `/api/v1/attachments?recordType=document&recordId=${encodeURIComponent(selectedID)}`,
      ),
    ])
      .then(([history, files]) => {
        setRevisions(history.revisions ?? []);
        if (attachmentVersion.current === requestVersion)
          setAttachments(files.attachments ?? []);
      })
      .catch(() => {
        setRevisions([]);
        if (attachmentVersion.current === requestVersion) setAttachments([]);
      });
  }, [selectedID]);

  function openCreate() {
    setDraft({ title: "", body: "", links: [] });
    setFormError("");
    setEditorMode("create");
  }

  function openEdit() {
    if (!selected) return;
    setDraft({
      title: selected.title,
      body: selected.body,
      links: selected.links ?? [],
    });
    setFormError("");
    setEditorMode("edit");
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    try {
      const target =
        editorMode === "edit" && selected
          ? `/api/v1/documents/${selected.id}`
          : "/api/v1/documents";
      const saved = await api<Document>(target, {
        method: editorMode === "edit" ? "PATCH" : "POST",
        body: JSON.stringify(draft),
      });
      setItems((current) =>
        editorMode === "edit"
          ? current.map((item) => (item.id === saved.id ? saved : item))
          : [saved, ...current],
      );
      setSelectedID(saved.id);
      setLinkOptions((current) =>
        editorMode === "edit"
          ? current.map((item) =>
              item.type === "document" && item.id === saved.id
                ? { ...item, label: saved.title }
                : item,
            )
          : [
              ...current,
              { type: "document", id: saved.id, label: saved.title },
            ],
      );
      setEditorMode(null);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save document",
      );
    } finally {
      setSaving(false);
    }
  }

  async function deleteDocument() {
    if (!deleting) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/documents/${deleting.id}`, { method: "DELETE" });
      const remaining = items.filter((item) => item.id !== deleting.id);
      setItems(remaining);
      setSelectedID(remaining[0]?.id ?? "");
      setDeleting(null);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not delete document",
      );
    } finally {
      setSaving(false);
    }
  }

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    data.set("kind", "attachment");
    data.set("recordType", "document");
    data.set("recordId", selected.id);
    setFormError("");
    const response = await fetch("/api/v1/attachments", {
      method: "POST",
      headers: { "X-Kosmos-CSRF": "1" },
      body: data,
    });
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as {
        error?: { message?: string };
      };
      setFormError(body.error?.message ?? "Could not upload file");
      return;
    }
    const attachment = (await response.json()) as Attachment;
    attachmentVersion.current += 1;
    setAttachments((current) => [attachment, ...current]);
    form.reset();
  }

  async function deleteAttachment(item: Attachment) {
    await api(`/api/v1/attachments/${item.id}`, { method: "DELETE" });
    attachmentVersion.current += 1;
    setAttachments((current) =>
      current.filter((candidate) => candidate.id !== item.id),
    );
  }

  if (loading) return <LoadingState label="Opening your documents" />;
  if (error) return <ErrorState message={error} retry={load} />;

  return (
    <>
      <Page
        eyebrow="Knowledge"
        title="Documents"
        detail="Write in Markdown, read it like a polished document, and keep the why beside the work."
        action={
          <button className="primary-button" onClick={openCreate}>
            <FilePlus2 size={17} /> New document
          </button>
        }
      >
        {items.length ? (
          <div className="document-layout">
            <aside className="document-list" aria-label="Documents">
              {items.map((item) => (
                <button
                  className={item.id === selectedID ? "active" : ""}
                  key={item.id}
                  onClick={() => setSelectedID(item.id)}
                >
                  <strong>{item.title}</strong>
                  <small>Updated {shortDate(item.updatedAt)}</small>
                </button>
              ))}
            </aside>
            <section className="document-sheet">
              {selected && (
                <>
                  <header>
                    <div>
                      <p className="eyebrow">
                        Document · Revision {selected.revision || 1}
                      </p>
                      <h2>{selected.title}</h2>
                      <small>Updated {shortDate(selected.updatedAt)}</small>
                    </div>
                    <div className="document-actions">
                      <button className="secondary-button" onClick={openEdit}>
                        <Edit3 size={16} /> Edit
                      </button>
                      <button
                        className="icon-button danger-icon"
                        aria-label={`Delete ${selected.title}`}
                        onClick={() => setDeleting(selected)}
                      >
                        <Trash2 size={17} />
                      </button>
                    </div>
                  </header>
                  {!!selected.links?.length && (
                    <div className="linked-records">
                      {selected.links.map((link) => (
                        <a
                          href={recordHref(link)}
                          key={`${link.type}:${link.id}`}
                        >
                          {linkLabel(link, linkOptions)}
                        </a>
                      ))}
                    </div>
                  )}
                  <article className="markdown">
                    <EmbeddedMarkdown
                      body={
                        selected.body ||
                        "_This document is empty. Choose Edit to start writing._"
                      }
                      attachments={attachments}
                    />
                  </article>
                  <DocumentAttachments
                    items={attachments}
                    onDelete={deleteAttachment}
                  />
                  <form className="document-upload" onSubmit={upload}>
                    <label>
                      <Paperclip size={16} /> Attach a file
                      <input
                        name="file"
                        type="file"
                        accept="image/jpeg,image/png,image/webp,application/pdf,text/plain"
                        required
                      />
                    </label>
                    <button className="secondary-button">
                      <UploadCloud size={16} /> Upload
                    </button>
                  </form>
                  {!!revisions.length && (
                    <p className="revision-note">
                      <History size={15} /> {revisions.length} prior version
                      {revisions.length === 1 ? "" : "s"} safely retained
                    </p>
                  )}
                </>
              )}
            </section>
          </div>
        ) : (
          <EmptyState
            title="No documents yet"
            detail="Create a handbook, client brief, checklist, or anything else worth remembering."
            action={
              <button className="primary-button" onClick={openCreate}>
                <FilePlus2 size={17} /> Create your first document
              </button>
            }
          />
        )}
      </Page>
      {editorMode && (
        <DocumentEditor
          mode={editorMode}
          draft={draft}
          options={linkOptions}
          saving={saving}
          error={formError}
          onDraft={setDraft}
          onSave={save}
          onClose={() => setEditorMode(null)}
        />
      )}
      {deleting && (
        <Modal
          eyebrow="Knowledge"
          title="Delete this document?"
          onClose={() => setDeleting(null)}
        >
          <p className="muted-copy">
            {deleting.title} and its revision history will be permanently
            deleted. Attached files remain available until removed separately.
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
              Keep document
            </button>
            <button
              className="danger-button"
              onClick={deleteDocument}
              disabled={saving}
            >
              <Trash2 size={16} /> {saving ? "Deleting..." : "Delete document"}
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}

function DocumentEditor({
  mode,
  draft,
  options,
  saving,
  error,
  onDraft,
  onSave,
  onClose,
}: {
  mode: "create" | "edit";
  draft: Draft;
  options: LinkOption[];
  saving: boolean;
  error: string;
  onDraft: (draft: Draft) => void;
  onSave: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}) {
  return (
    <div
      className="document-workspace"
      role="dialog"
      aria-modal="true"
      aria-label={mode === "create" ? "Create document" : "Edit document"}
    >
      <form onSubmit={onSave}>
        <header>
          <div>
            <p className="eyebrow">Knowledge workspace</p>
            <input
              aria-label="Title"
              value={draft.title}
              onChange={(event) =>
                onDraft({ ...draft, title: event.target.value })
              }
              maxLength={160}
              placeholder="Untitled document"
              required
              autoFocus
            />
          </div>
          <div className="document-actions">
            <button
              type="button"
              className="secondary-button"
              onClick={onClose}
            >
              <X size={16} /> Cancel
            </button>
            <button className="primary-button" disabled={saving}>
              <Save size={16} />{" "}
              {saving
                ? "Saving..."
                : mode === "create"
                  ? "Create document"
                  : "Save document"}
            </button>
          </div>
        </header>
        <main>
          <textarea
            className="sr-only"
            aria-label="Start writing in Markdown"
            value={draft.body}
            onChange={(event) =>
              onDraft({ ...draft, body: event.target.value })
            }
          />
          <CodeMirror
            value={draft.body}
            height="100%"
            minHeight="320px"
            extensions={[
              markdown(),
              editorTheme,
              EditorView.lineWrapping,
              EditorView.contentAttributes.of({
                "aria-label": "Markdown editor",
              }),
            ]}
            onChange={(body) => onDraft({ ...draft, body })}
            placeholder="# Start writing"
          />
        </main>
        <footer>
          <LinkFields
            links={draft.links}
            options={options}
            onChange={(links) => onDraft({ ...draft, links })}
          />
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <small>
            Markdown is highlighted with line numbers. Add an uploaded file
            using <code>[[filename]]</code>.
          </small>
        </footer>
      </form>
    </div>
  );
}

function DocumentAttachments({
  items,
  onDelete,
}: {
  items: Attachment[];
  onDelete: (item: Attachment) => void;
}) {
  if (!items.length) return null;
  return (
    <section className="document-files">
      <h3>Files</h3>
      <div className="file-list">
        {items.map((item) => (
          <article className="record-row compact" key={item.id}>
            <Paperclip size={17} />
            <span>
              <strong>{item.fileName}</strong>
              <small>
                Embed with <code>[[{item.fileName}]]</code>
              </small>
            </span>
            <a
              className="icon-button"
              href={item.downloadUrl}
              aria-label={`Download ${item.fileName}`}
            >
              <Download size={16} />
            </a>
            <button
              className="icon-button danger-icon"
              aria-label={`Delete ${item.fileName}`}
              onClick={() => onDelete(item)}
            >
              <Trash2 size={16} />
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}

function EmbeddedMarkdown({
  body,
  attachments,
}: {
  body: string;
  attachments: Attachment[];
}) {
  const parts = useMemo(() => body.split(/(\[\[[^\]]+\]\])/g), [body]);
  return (
    <>
      {parts.map((part, index) => {
        const match = /^\[\[([^\]]+)\]\]$/.exec(part);
        if (!match) return <ReactMarkdown key={index}>{part}</ReactMarkdown>;
        const file = attachments.find((item) => item.fileName === match[1]);
        if (!file)
          return (
            <code className="missing-embed" key={index}>
              {part}
            </code>
          );
        const contentType = file.contentType.toLowerCase();
        const fileName = file.fileName.toLowerCase();
        if (
          contentType.startsWith("image/") ||
          /\.(jpe?g|png|webp)$/.test(fileName)
        )
          return (
            <figure className="document-embed" key={file.id}>
              <img src={file.viewUrl} alt={file.fileName} />
              <figcaption>
                <a href={file.downloadUrl}>{file.fileName}</a>
              </figcaption>
            </figure>
          );
        if (contentType === "application/pdf" || fileName.endsWith(".pdf"))
          return (
            <figure className="document-embed pdf" key={file.id}>
              <iframe src={file.viewUrl} title={file.fileName} />
              <figcaption>
                <a href={file.downloadUrl}>Download {file.fileName}</a>
              </figcaption>
            </figure>
          );
        return (
          <a className="attachment-link" href={file.downloadUrl} key={file.id}>
            <Download size={15} /> {file.fileName}
          </a>
        );
      })}
    </>
  );
}

function LinkFields({
  links = [],
  options,
  onChange,
}: {
  links?: RecordLink[];
  options: LinkOption[];
  onChange: (links: RecordLink[]) => void;
}) {
  const type = links[0]?.type ?? "";
  const id = links[0]?.id ?? "";
  const available = options.filter((option) => option.type === type);
  return (
    <div className="field-grid">
      <label>
        Link to
        <select
          value={type}
          onChange={(event) =>
            onChange(
              event.target.value
                ? [{ type: event.target.value as RecordLink["type"], id: "" }]
                : [],
            )
          }
        >
          <option value="">Nothing yet</option>
          <option value="account">Account</option>
          <option value="contact">Contact</option>
          <option value="opportunity">Opportunity</option>
          <option value="cost">Cost</option>
          <option value="document">Document</option>
        </select>
      </label>
      <label>
        Linked record
        <select
          value={id}
          disabled={!type}
          onChange={(event) =>
            onChange(
              type && event.target.value
                ? [{ type, id: event.target.value }]
                : [],
            )
          }
        >
          <option value="">
            {type ? "Choose a record" : "Choose what to link first"}
          </option>
          {available.map((option) => (
            <option value={option.id} key={`${option.type}:${option.id}`}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}

function recordHref(link: RecordLink) {
  if (link.type === "account") return `/accounts/${link.id}`;
  if (link.type === "contact") return `/contacts/${link.id}`;
  if (link.type === "opportunity") return "/opportunities";
  if (link.type === "cost") return "/operations";
  return "/documents";
}

function linkLabel(link: RecordLink, options: LinkOption[]) {
  const label =
    options.find((option) => option.type === link.type && option.id === link.id)
      ?.label ?? "Linked record";
  return `${link.type[0].toUpperCase()}${link.type.slice(1)} · ${label}`;
}
