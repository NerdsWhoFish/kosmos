import { useEffect, useState, type FormEvent } from "react";
import { FileSignature, Plus, UploadCloud } from "lucide-react";
import { Page } from "../components/Page";
import { EmptyState, ErrorState, LoadingState } from "../components/States";
import { shortDate } from "../api";
import { signingAPI, signingError as errorMessage, type SigningRequest } from "./signingApi";
import { SigningEditor } from "./SigningEditor";
import { Status } from "./SigningFields";
import "./signing.css";

const endpoint = "/api/v1/signing-requests";

export function Signing({
  id = "",
  navigate,
}: {
  id?: string;
  navigate: (path: string) => void;
}) {
  const [requests, setRequests] = useState<SigningRequest[]>([]);
  const [request, setRequest] = useState<SigningRequest>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [title, setTitle] = useState("");
  const [file, setFile] = useState<File>();
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let current = true;
    setLoading(true);
    setError("");
    setRequest(undefined);
    const load = id
      ? signingAPI<SigningRequest>(`${endpoint}/${encodeURIComponent(id)}`)
      : signingAPI<{ requests: SigningRequest[] }>(endpoint);
    load
      .then((result) => {
        if (!current) return;
        if ("requests" in result) setRequests(result.requests ?? []);
        else setRequest(result);
      })
      .catch((error) => {
        if (current) setError(errorMessage(error));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [id, attempt]);

  async function upload(event: FormEvent) {
    event.preventDefault();
    if (!file || busy) return;
    setBusy(true);
    setError("");
    try {
      const form = new FormData();
      form.append("file", file);
      form.append(
        "title",
        title.trim() || file.name.replace(/\.pdf$/i, "").slice(0, 160),
      );
      const created = await signingAPI<SigningRequest>(endpoint, undefined, {
        method: "POST",
        body: form,
      });
      navigate(`/documents/signing/${created.id}`);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <LoadingState label="Loading signing requests" />;
  if (id && request)
    return (
      <SigningEditor key={request.id} initial={request} navigate={navigate} />
    );
  return (
    <Page
      eyebrow="Documents / Signing"
      title="Get it signed."
      detail="Upload a PDF, place your fields, and share a private signing link."
      action={
        <button
          className="secondary-button"
          onClick={() => navigate("/documents")}
        >
          All documents
        </button>
      }
    >
      {error && (
        <ErrorState
          message={error}
          retry={() => setAttempt((value) => value + 1)}
        />
      )}
      {!id && (
        <>
          <form className="signing-upload signing-card" onSubmit={upload} aria-busy={busy}>
            <div className="signing-upload-heading">
              <UploadCloud size={28} />
              <div>
                <h2>Start with your PDF</h2>
                <p>
                  We automatically prepare PDFs for signing and keep your original upload.
                </p>
              </div>
            </div>
            <label>
              Document title
              <input
                maxLength={160}
                disabled={busy}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="e.g. Website services agreement"
              />
            </label>
            <label>
              PDF document (up to 10 MB, 50 pages)
              <input
                type="file"
                disabled={busy}
                accept="application/pdf,.pdf"
                required
                onChange={(event) => setFile(event.target.files?.[0])}
              />
            </label>
            <button className="primary-button" disabled={busy || !file}>
              <Plus size={17} />
              {busy ? "Preparing PDF…" : "Upload and place fields"}
            </button>
          </form>
          <div className="signing-section-heading">
            <h2>Signing requests</h2>
            <button
              className="secondary-button"
              onClick={() => setAttempt((value) => value + 1)}
            >
              Refresh status
            </button>
          </div>
          {requests.length ? (
            <div className="signing-request-list">
              {requests.map((item) => (
                <button
                  key={item.id}
                  className="signing-request-row"
                  onClick={() => navigate(`/documents/signing/${item.id}`)}
                >
                  <FileSignature size={24} />
                  <span>
                    <strong>{item.title}</strong>
                    <small>
                      {item.signerEmail || item.fileName} ·{" "}
                      {shortDate(item.createdAt)}
                    </small>
                  </span>
                  <Status request={item} />
                </button>
              ))}
            </div>
          ) : (
            <EmptyState
              title="Your next agreement starts here"
              detail="Upload your first PDF above. Drafts, active links, and signed copies will live here."
            />
          )}
        </>
      )}
    </Page>
  );
}
