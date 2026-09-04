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
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import {
  Download,
  Edit3,
  FilePlus2,
  History,
  Link2,
  Paperclip,
  Save,
  Trash2,
  UploadCloud,
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
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { WorkflowPage } from "../components/WorkflowPage";
import { ResourceRoute } from "../routing";

type LinkOption = RecordLink & { label: string };
type Draft = { title: string; body: string; links: RecordLink[] };

const editorTheme = EditorView.theme(
  {
    "&": {
      backgroundColor: "var(--theme-surface)",
      color: "var(--theme-text)",
    },
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
      backgroundColor:
        "color-mix(in srgb, var(--theme-accent) 10%, transparent)",
    },
    ".cm-selectionBackground, ::selection": {
      backgroundColor:
        "color-mix(in srgb, var(--theme-accent) 32%, transparent) !important",
    },
  },
  { dark: true },
);

const markdownHighlight = HighlightStyle.define([
  { tag: tags.heading, color: "var(--theme-link)", fontWeight: "700" },
  { tag: [tags.link, tags.url], color: "var(--theme-link)" },
  { tag: tags.strong, color: "var(--theme-success)", fontWeight: "700" },
  { tag: tags.emphasis, color: "var(--theme-warning)", fontStyle: "italic" },
  { tag: tags.monospace, color: "var(--theme-success)" },
  { tag: [tags.meta, tags.punctuation], color: "var(--theme-muted)" },
]);

export function Documents({
  route,
  accountID,
  navigate,
}: {
  route: ResourceRoute;
  accountID: string;
  navigate: (path: string) => void;
}) {
  const [items, setItems] = useState<Document[]>([]);
  const [draft, setDraft] = useState<Draft>({ title: "", body: "", links: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [revisions, setRevisions] = useState<DocumentRevision[]>([]);
  const [linkOptions, setLinkOptions] = useState<LinkOption[]>([]);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [deletingAttachmentID, setDeletingAttachmentID] = useState("");
  const [copyNotice, setCopyNotice] = useState("");
  const attachmentVersion = useRef(0);
  const selected = items.find((item) => item.id === route.id);

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
    const requestVersion = ++attachmentVersion.current;
    if (!route.id) {
      setAttachments([]);
      setRevisions([]);
      return;
    }
    Promise.all([
      api<{ revisions: DocumentRevision[] }>(
        `/api/v1/documents/${route.id}/revisions`,
      ),
      api<{ attachments: Attachment[] }>(
        `/api/v1/attachments?recordType=document&recordId=${encodeURIComponent(route.id)}`,
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
  }, [route.id]);

  useEffect(() => {
    if (route.action === "new") {
      setDraft({
        title: "",
        body: "",
        links: accountID ? [{ type: "account", id: accountID }] : [],
      });
      setFormError("");
    } else if (route.action === "edit" && selected) {
      setDraft({
        title: selected.title,
        body: selected.body,
        links: selected.links ?? [],
      });
      setFormError("");
    }
  }, [accountID, route.action, route.id, selected]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    try {
      const target =
        route.action === "edit" && selected
          ? `/api/v1/documents/${selected.id}`
          : "/api/v1/documents";
      const saved = await api<Document>(target, {
        method: route.action === "edit" ? "PATCH" : "POST",
        body: JSON.stringify(draft),
      });
      setItems((current) =>
        route.action === "edit"
          ? current.map((item) => (item.id === saved.id ? saved : item))
          : [saved, ...current],
      );
      setLinkOptions((current) =>
        route.action === "edit"
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
      navigate(`/documents/${saved.id}`);
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not save document",
      );
    } finally {
      setSaving(false);
    }
  }

  async function deleteDocument() {
    if (!selected) return;
    setSaving(true);
    setFormError("");
    try {
      await api(`/api/v1/documents/${selected.id}`, { method: "DELETE" });
      setItems((current) => current.filter((item) => item.id !== selected.id));
      navigate("/documents");
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
    const fileInput = form.elements.namedItem("file") as HTMLInputElement;
    if (fileInput.files?.[0]) data.set("file", fileInput.files[0]);
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
    try {
      await api(`/api/v1/attachments/${item.id}`, { method: "DELETE" });
      attachmentVersion.current += 1;
      setAttachments((current) =>
        current.filter((candidate) => candidate.id !== item.id),
      );
      setDeletingAttachmentID("");
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "Could not delete file",
      );
    }
  }

  async function copyDocumentLink() {
    if (!selected) return;
    await navigator.clipboard.writeText(
      `${window.location.origin}/documents/${encodeURIComponent(selected.id)}`,
    );
    setCopyNotice("Document link copied.");
  }

  if (loading) return <LoadingState label="Opening your documents" />;
  if (error) return <ErrorState message={error} retry={load} />;

  if (route.action === "new" || route.action === "edit") {
    if (route.action === "edit" && !selected)
      return <ErrorState message="That document could not be found." />;
    return (
      <DocumentEditor
        mode={route.action}
        draft={draft}
        options={linkOptions.filter(
          (option) => option.type !== "document" || option.id !== selected?.id,
        )}
        saving={saving}
        error={formError}
        onDraft={setDraft}
        onSave={save}
        onClose={() =>
          navigate(selected ? `/documents/${selected.id}` : "/documents")
        }
      />
    );
  }

  if (route.action === "delete") {
    if (!selected)
      return <ErrorState message="That document could not be found." />;
    return (
      <WorkflowPage
        eyebrow="Knowledge"
        title="Delete this document?"
        detail={`${selected.title} and its revision history will be permanently deleted. Attached files remain until removed separately.`}
        backLabel="Keep document"
        onBack={() => navigate(`/documents/${selected.id}`)}
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
            onClick={() => navigate(`/documents/${selected.id}`)}
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
      </WorkflowPage>
    );
  }

  if (route.id) {
    if (!selected)
      return <ErrorState message="That document could not be found." />;
    return (
      <Page
        eyebrow="Knowledge"
        title={selected.title}
        detail={`Revision ${selected.revision || 1} · Updated ${shortDate(selected.updatedAt)}`}
        action={
          <div className="button-row page-actions">
            <button className="secondary-button" onClick={copyDocumentLink}>
              <Link2 size={16} /> Copy link
            </button>
            <button
              className="secondary-button"
              onClick={() => navigate(`/documents/${selected.id}/edit`)}
            >
              <Edit3 size={16} /> Edit
            </button>
            <button
              className="danger-button"
              onClick={() => navigate(`/documents/${selected.id}/delete`)}
            >
              <Trash2 size={16} /> Delete
            </button>
          </div>
        }
      >
        <button className="back-button" onClick={() => navigate("/documents")}>
          ← All documents
        </button>
        {copyNotice && (
          <p className="inline-notice" role="status">
            {copyNotice}
          </p>
        )}
        <section className="document-sheet document-detail-sheet">
          {!!selected.links?.length && (
            <div className="linked-records">
              {selected.links.map((link) => (
                <a href={recordHref(link)} key={`${link.type}:${link.id}`}>
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
        </section>
        <section className="panel document-attachment-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Files</p>
              <h2>Attachments</h2>
              <p className="muted-copy">
                Add supporting files, then reference them with normal Markdown
                links or images.
              </p>
            </div>
          </div>
          <DocumentAttachments
            items={attachments}
            deletingID={deletingAttachmentID}
            onRequestDelete={(item) => setDeletingAttachmentID(item.id)}
            onCancelDelete={() => setDeletingAttachmentID("")}
            onDelete={deleteAttachment}
          />
          <form className="document-upload" onSubmit={upload}>
            <label className="file-picker">
              <span>
                <Paperclip size={18} /> Choose a file
              </span>
              <input
                name="file"
                type="file"
                accept="image/jpeg,image/png,image/webp,image/svg+xml,application/pdf,text/plain,text/markdown,application/json,text/css,.md,.markdown,.json,.css,.svg"
                required
              />
            </label>
            <button className="primary-button">
              <UploadCloud size={16} /> Upload attachment
            </button>
          </form>
          {formError && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
        </section>
        {!!revisions.length && (
          <p className="revision-note">
            <History size={15} /> {revisions.length} prior version
            {revisions.length === 1 ? "" : "s"} safely retained
          </p>
        )}
      </Page>
    );
  }

  return (
    <Page
      eyebrow="Knowledge"
      title="Documents"
      detail="Write in Markdown, read it like a polished document, and keep the why beside the work."
      action={
        <button
          className="primary-button"
          onClick={() => navigate("/documents/new")}
        >
          <FilePlus2 size={17} /> New document
        </button>
      }
    >
      {items.length ? (
        <div className="document-index" aria-label="Documents">
          {items.map((item) => (
            <a
              className="document-index-row"
              href={`/documents/${encodeURIComponent(item.id)}`}
              key={item.id}
              onClick={(event) => {
                event.preventDefault();
                navigate(`/documents/${item.id}`);
              }}
            >
              <span>
                <strong>{item.title}</strong>
                <small>Updated {shortDate(item.updatedAt)}</small>
              </span>
              <span className="document-open-label">Open document →</span>
            </a>
          ))}
        </div>
      ) : (
        <EmptyState
          title="No documents yet"
          detail="Create a handbook, client brief, checklist, or anything else worth remembering."
          action={
            <button
              className="primary-button"
              onClick={() => navigate("/documents/new")}
            >
              <FilePlus2 size={17} /> Create your first document
            </button>
          }
        />
      )}
    </Page>
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
  mode: "new" | "edit";
  draft: Draft;
  options: LinkOption[];
  saving: boolean;
  error: string;
  onDraft: (draft: Draft) => void;
  onSave: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}) {
  return (
    <WorkflowPage
      eyebrow="Knowledge workspace"
      title={mode === "new" ? "New document" : "Edit document"}
      detail="Write without fighting a cramped overlay. Your editor and controls scroll with the page."
      backLabel={mode === "new" ? "All documents" : "Back to document"}
      onBack={onClose}
    >
      <div className="document-workspace">
        <form onSubmit={onSave}>
          <header>
            <div>
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
                Cancel
              </button>
              <button className="primary-button" disabled={saving}>
                <Save size={16} />{" "}
                {saving
                  ? "Saving..."
                  : mode === "new"
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
              theme={editorTheme}
              extensions={[
                markdown(),
                syntaxHighlighting(markdownHighlight),
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
              Use standard Markdown for uploaded files:{" "}
              <code>![Description](image.png)</code> or{" "}
              <code>[Download file](guide.pdf)</code>.
            </small>
          </footer>
        </form>
      </div>
    </WorkflowPage>
  );
}

function DocumentAttachments({
  items,
  deletingID,
  onRequestDelete,
  onCancelDelete,
  onDelete,
}: {
  items: Attachment[];
  deletingID: string;
  onRequestDelete: (item: Attachment) => void;
  onCancelDelete: () => void;
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
                Markdown:{" "}
                <code>
                  {isImageAttachment(item)
                    ? `![Description](${item.fileName})`
                    : `[Download ${item.fileName}](${item.fileName})`}
                </code>
              </small>
            </span>
            <a
              className="icon-button"
              href={item.downloadUrl}
              aria-label={`Download ${item.fileName}`}
            >
              <Download size={16} />
            </a>
            {deletingID === item.id ? (
              <span className="inline-confirm">
                <strong>Delete file?</strong>
                <button className="text-button" onClick={onCancelDelete}>
                  Keep
                </button>
                <button
                  className="text-button danger-text"
                  onClick={() => onDelete(item)}
                >
                  Delete
                </button>
              </span>
            ) : (
              <button
                className="icon-button danger-icon"
                aria-label={`Delete ${item.fileName}`}
                onClick={() => onRequestDelete(item)}
              >
                <Trash2 size={16} />
              </button>
            )}
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
        if (!match)
          return (
            <ReactMarkdown
              components={{
                img: ({ src, alt }) => {
                  const file = resolveAttachment(src, attachments);
                  return (
                    <img
                      src={file?.viewUrl ?? src}
                      alt={alt ?? file?.fileName ?? ""}
                    />
                  );
                },
                a: ({ href, children }) => {
                  const file = resolveAttachment(href, attachments);
                  return <a href={file?.downloadUrl ?? href}>{children}</a>;
                },
              }}
              key={index}
            >
              {part}
            </ReactMarkdown>
          );
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

function resolveAttachment(
  destination: string | undefined,
  attachments: Attachment[],
) {
  if (
    !destination ||
    destination.startsWith("#") ||
    /^[a-z][a-z0-9+.-]*:/i.test(destination) ||
    destination.startsWith("//")
  )
    return undefined;
  const path = destination.split(/[?#]/, 1)[0];
  let basename = path.split("/").filter(Boolean).at(-1) ?? "";
  try {
    basename = decodeURIComponent(basename);
  } catch {
    return undefined;
  }
  return attachments.find(
    (item) => item.fileName.toLowerCase() === basename.toLowerCase(),
  );
}

function isImageAttachment(item: Attachment) {
  return (
    item.contentType.toLowerCase().startsWith("image/") ||
    /\.(jpe?g|png|webp|svg)$/i.test(item.fileName)
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
  return `/documents/${link.id}`;
}

function linkLabel(link: RecordLink, options: LinkOption[]) {
  const label =
    options.find((option) => option.type === link.type && option.id === link.id)
      ?.label ?? "Linked record";
  return `${link.type[0].toUpperCase()}${link.type.slice(1)} · ${label}`;
}
