import { useCallback, useEffect, useState, type FormEvent } from "react";
import { CheckCircle2, Download, FileSignature } from "lucide-react";
import { ErrorState, LoadingState } from "../components/States";
import { SigningPDF } from "../components/SigningPDF";
import { APIRequestError, shortDate } from "../api";
import { consentText, downloadPDF, signingAPI, signingCredential, signingError as errorMessage, type SigningRequest } from "./signingApi";
import { FieldOverlay, PageControls } from "./SigningFields";
import { SigningDeadline, useSigningDeadline } from "./SigningDeadline";
import "./signing.css";


export function PublicSigning() {
  const [credential, setCredential] = useState(() =>
    signingCredential(window.location.hash),
  );
  const [request, setRequest] = useState<SigningRequest>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [page, setPage] = useState(1);
  const [signerName, setSignerName] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [consent, setConsent] = useState(false);
  const [documentReady, setDocumentReady] = useState(false);
  const [pageCount, setPageCount] = useState(1);
  const [attempt, setAttempt] = useState(0);
  const [unavailable, setUnavailable] = useState(false);
  const currentSigner = request?.signers?.find((signer) => signer.id === request.currentSignerId);
  const signed = request?.status === "completed" || !!currentSigner?.signedAt;
  const waiting = request?.signers?.filter((signer) => !signer.signedAt) ?? [];
  const fields = request?.fields.filter((field) => !request.currentSignerId || field.signerId === request.currentSignerId) ?? [];
  const expiresAt = request?.accessExpiresAt ?? (request?.status === "completed" ? request.postSignExpiresAt : request?.expiresAt);
  const { expired, remaining } = useSigningDeadline(expiresAt);
  const onPDFReady = useCallback(
    (ready: boolean) => setDocumentReady(ready),
    [],
  );
  const path = credential
    ? `/api/v1/signing/${encodeURIComponent(credential.id)}`
    : "";

  useEffect(() => {
    const update = () => setCredential(signingCredential(window.location.hash));
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  useEffect(() => {
    let current = true;
    setLoading(true);
    setError("");
    setRequest(undefined);
    setUnavailable(false);
    setValues({});
    setConsent(false);
    setPage(1);
    if (!credential) {
      setLoading(false);
      return;
    }
    signingAPI<SigningRequest>(path, credential.token)
      .then((result) => {
        if (current) {
          setRequest(result);
          setPageCount(result.pages.length);
          setSignerName(result.signers?.find((signer) => signer.id === result.currentSignerId)?.name || result.signerName || "");
        }
      })
      .catch((error) => {
        if (current) {
          if (error instanceof APIRequestError && [404, 410].includes(error.status)) setUnavailable(true);
          else setError(errorMessage(error));
        }
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [credential, path, attempt]);

  useEffect(() => {
    if (!credential || !request || expired || unavailable) return;
    let current = true;
    const refresh = () => {
      if (document.visibilityState === "hidden") return;
      signingAPI<SigningRequest>(path, credential.token).then((result) => {
        if (current) setRequest(result);
      }).catch((error) => {
        if (current && error instanceof APIRequestError && [404, 410].includes(error.status)) setUnavailable(true);
      });
    };
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      current = false;
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [credential, path, request, expired, unavailable]);

  const accessEnded = () => unavailable || (expiresAt !== undefined && new Date(expiresAt).getTime() <= Date.now());

  async function download(completed: boolean) {
    if (!request || !credential || busy || accessEnded()) return;
    setBusy(true);
    setError("");
    try {
      await downloadPDF(
        `${path}/pdf${completed ? "?completed=true" : ""}`,
        completed ? `signed-${request.fileName}` : request.fileName,
        credential.token,
      );
      if (!completed) setDocumentReady(true);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  async function complete(event: FormEvent) {
    event.preventDefault();
    if (!credential || !request || busy || !consent || !documentReady || accessEnded()) return;
    setBusy(true);
    setError("");
    try {
      setRequest(
        await signingAPI<SigningRequest>(`${path}/complete`, credential.token, {
          method: "POST",
          body: JSON.stringify({
            values,
            signerName: signerName.trim(),
            consent: true,
          }),
        }),
      );
      setPage(1);
      window.scrollTo({ top: 0, behavior: "auto" });
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  const available = request?.status === "pending" && !signed && !expired;
  const requiredComplete =
    !!request &&
    fields
      .filter(
        (field) =>
          field.required &&
          (field.type === "signature" || field.type === "text"),
      )
      .every((field) => values[field.id]?.trim());

  return (
    <main className="public-signing">
      <header className="signing-public-brand">
        <FileSignature size={25} />
        <strong>Kosmos</strong>
        <span>Document signing</span>
      </header>
      {loading ? (
        <LoadingState label="Opening your document" />
      ) : !credential ? (
        <div className="signing-card">
          <h1>This signing link is incomplete.</h1>
          <p>
            Open the full link from the sender, including everything after the #
            symbol.
          </p>
        </div>
      ) : unavailable ? (
        <section className="signing-card">
          <h1>This document link is no longer available.</h1>
          <p>It may have expired or been replaced. Ask the sender for a new link.</p>
        </section>
      ) : (
        <>
          {error && (
            <ErrorState
              message={error}
              retry={
                !request ? () => setAttempt((value) => value + 1) : undefined
              }
            />
          )}
          {request && (
            <>
              {signed ? (
                <section className="signing-complete" aria-live="polite">
                  <CheckCircle2 size={48} />
                  <p className="eyebrow">{request.status === "completed" ? "Signing complete" : "Your signature is saved"}</p>
                  <h1>You’re all signed.</h1>
                  <p>{request.title}</p>
                  <p>{request.status === "completed" ? "Your signature is saved." : `Waiting for ${waiting.map((signer) => signer.name).join(", ")} to sign.`}</p>
                  {expired ? (
                    <div role="status">
                      <h2>This download link has expired.</h2>
                      <p>{request.status === "completed" ? "Ask the sender for a new download link to your signed document." : "Ask the sender for a copy, or a new download link after everyone signs."}</p>
                    </div>
                  ) : <>
                  <p>{request.status === "completed" ? "Download your completed document and signing record for your files." : "This copy includes the signatures collected so far. The sender can share a new link to the final document after everyone signs."}</p>
                  {expiresAt && <SigningDeadline expiresAt={expiresAt} remaining={remaining} />}
                  <button
                    className="primary-button"
                    disabled={busy}
                    onClick={() => download(true)}
                  >
                    <Download size={18} />
                    {busy ? "Preparing download…" : request.status === "completed" ? "Download signed PDF" : "Download current signed copy"}
                  </button>
                  </>}
                </section>
              ) : (
                <header className="signing-public-heading">
                  <p className="eyebrow">Review & sign</p>
                  <h1>{request.title}</h1>
                  <p>
                    Review the document, complete your fields, then confirm your
                    signature.
                  </p>
                </header>
              )}
              {signed && expired ? null : !available && !signed ? (
                <section className="signing-card">
                  <h2>
                    {expired
                      ? "This signing link has expired."
                      : "This document is no longer available for signing."}
                  </h2>
                  <p>Ask the sender for a new signing link.</p>
                </section>
              ) : (
                <div className="signing-workbench">
                  <div className="signing-document-column">
                    {available && request.flattened && (
                      <p className="signing-notice">
                        This document was prepared as a fixed copy for signing. Review each page before you sign.
                      </p>
                    )}
                    <PageControls
                      page={page}
                      count={pageCount}
                      onChange={setPage}
                    />
                    <SigningPDF
                      key={signed ? request.signedSHA256 ?? request.updatedAt : request.id}
                      path={`${path}/pdf${signed ? "?completed=true" : ""}`}
                      token={credential.token}
                      page={page}
                      {...(request.pages[page - 1] ?? request.pages[0])}
                      onReady={onPDFReady}
                      onPageCount={setPageCount}
                    >
                      {!signed &&
                        fields
                          .filter((field) => field.page === page)
                          .map((field) => (
                            <FieldOverlay
                              key={field.id}
                              field={field}
                              value={
                                field.type === "name"
                                  ? signerName
                                  : field.type === "date"
                                    ? "Date signed"
                                    : values[field.id]
                              }
                            />
                          ))}
                    </SigningPDF>
                  </div>
                  {available && (
                    <form
                      className="signing-card signing-signer-form"
                      onSubmit={complete}
                    >
                      <p className="eyebrow">Your signature</p>
                      <h2>Make it official.</h2>
                      <p>
                        For {currentSigner?.name || request.signerName}
                        {request.expiresAt
                          ? ` · Expires ${shortDate(request.expiresAt)}`
                          : ""}
                      </p>
                      <button
                        type="button"
                        className="secondary-button"
                        disabled={busy}
                        onClick={() => download(false)}
                      >
                        <Download size={16} />
                        Download document to review
                      </button>
                      <p className="signing-hint">After you sign, your link stays available for downloading for 15 minutes.</p>
                      <fieldset disabled={busy}>
                        <label>
                          Your full name
                          <input
                            required
                            maxLength={120}
                            autoComplete="name"
                            value={signerName}
                            onChange={(event) =>
                              setSignerName(event.target.value)
                            }
                          />
                        </label>
                        {fields.map((field, index) => (
                          <div key={field.id} className="signing-answer">
                            <label htmlFor={`answer-${field.id}`}>
                              {index + 1}. {field.label}
                              {field.required ? " *" : " (optional)"}
                              <small>Page {field.page}</small>
                            </label>
                            {field.type === "date" || field.type === "name" ? (
                              <p id={`answer-${field.id}`}>
                                {field.type === "date"
                                  ? "Filled with the date you sign (UTC)."
                                  : signerName || "Filled with your full name."}
                              </p>
                            ) : (
                              <input
                                id={`answer-${field.id}`}
                                className={
                                  field.type === "signature"
                                    ? "signing-typed-signature"
                                    : ""
                                }
                                required={field.required}
                                maxLength={200}
                                placeholder={
                                  field.type === "signature"
                                    ? "Type your signature"
                                    : "Enter your answer"
                                }
                                value={values[field.id] || ""}
                                onChange={(event) =>
                                  setValues((current) => ({
                                    ...current,
                                    [field.id]: event.target.value,
                                  }))
                                }
                              />
                            )}
                            <button
                              type="button"
                              className="text-button"
                              onClick={() => {
                                setPage(field.page);
                                document
                                  .querySelector(".signing-preview")
                                  ?.scrollIntoView({
                                    behavior: "auto",
                                    block: "start",
                                  });
                              }}
                            >
                              View on page {field.page}
                            </button>
                          </div>
                        ))}
                        <p className="signing-hint">
                          Typing your signature creates an electronic signature.
                          Date fields are filled when you finish.
                        </p>
                        <p id="signing-session-disclosure" className="signing-hint">
                          Your IP address, browser details, and approximate location will be recorded with your signature and shared with the sender{(request.signers?.length ?? 0) > 1 ? " and other signers" : ""}.
                        </p>
                        <label className="signing-checkbox signing-consent">
                          <input
                            required
                            type="checkbox"
                            aria-describedby="signing-session-disclosure"
                            checked={consent}
                            onChange={(event) =>
                              setConsent(event.target.checked)
                            }
                          />
                          <span>{consentText}</span>
                        </label>
                      </fieldset>
                      <button
                        className="primary-button signing-finish"
                        disabled={
                          busy ||
                          !consent ||
                          !signerName.trim() ||
                          !requiredComplete ||
                          !documentReady
                        }
                      >
                        <CheckCircle2 size={18} />
                        {busy
                          ? "Saving your signature…"
                          : "Agree & finish signing"}
                      </button>
                      {!documentReady && (
                        <p className="signing-hint">
                          Wait for the PDF to load or download the document
                          before signing.
                        </p>
                      )}
                      <p className="signing-hint">
                        The completed PDF includes a record of this signing.
                        Keep a copy for your files.
                      </p>
                    </form>
                  )}
                </div>
              )}
            </>
          )}
        </>
      )}
    </main>
  );
}
