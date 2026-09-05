import { useEffect, useState, type FormEvent } from "react";
import { Copy, Download, FileSignature, Plus, Save, Trash2 } from "lucide-react";
import { Page } from "../components/Page";
import { ErrorState } from "../components/States";
import { SigningPDF } from "../components/SigningPDF";
import { shortDate } from "../api";
import { boundedField, downloadPDF, fieldLabels, signingAPI, signingError as errorMessage, type SigningField, type SigningRequest } from "./signingApi";
import { FieldOverlay, PageControls, Status } from "./SigningFields";
import { SigningSessionDetails } from "./SigningSessionDetails";

const endpoint = "/api/v1/signing-requests";

export function SigningEditor({
  initial,
  navigate,
}: {
  initial: SigningRequest;
  navigate: (path: string) => void;
}) {
  const [request, setRequest] = useState(initial);
  const [fields, setFields] = useState(initial.fields ?? []);
  const [selectedID, setSelectedID] = useState("");
  const [focusFieldID, setFocusFieldID] = useState("");
  const [page, setPage] = useState(1);
  const [signerName, setSignerName] = useState(initial.signerName ?? "");
  const [signerEmail, setSignerEmail] = useState(initial.signerEmail ?? "");
  const [expiresDays, setExpiresDays] = useState(14);
  const [link, setLink] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const editable = request.status === "draft";
  const dirty =
    editable && JSON.stringify(fields) !== JSON.stringify(request.fields ?? []);
  const selected = fields.find((field) => field.id === selectedID);
  const path = `${endpoint}/${encodeURIComponent(request.id)}`;
  const [pageCount, setPageCount] = useState(request.pages.length);
  const dimensions = request.pages[page - 1] ?? request.pages[0];

  useEffect(() => {
    const prevent = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", prevent);
    return () => window.removeEventListener("beforeunload", prevent);
  }, [dirty]);

  function update(field: SigningField) {
    setFields((current) =>
      current.map((item) =>
          item.id === field.id ? boundedField(field, request.pages[field.page - 1]) : item,
      ),
    );
  }
  function add(type: SigningField["type"]) {
    const field: SigningField = {
      id: crypto.randomUUID(),
      type,
      label: fieldLabels[type],
      page,
      x: 0.12,
      y: Math.min(
        0.82,
        0.15 + fields.filter((item) => item.page === page).length * 0.07,
      ),
      width: type === "date" ? 0.24 : 0.35,
      height: 0.05,
      required: true,
    };
    setFields((current) => [...current, boundedField(field, dimensions)]);
    setSelectedID(field.id);
    setFocusFieldID(field.id);
    setNotice(
      `${fieldLabels[type]} added to page ${page}. Drag it into place, then drag a corner to resize.`,
    );
  }
  async function action(run: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await run();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!dirty) return request;
    const updated = await signingAPI<SigningRequest>(path, undefined, {
      method: "PUT",
      body: JSON.stringify({ revision: request.revision, fields }),
    });
    setRequest(updated);
    setFields(updated.fields);
    return updated;
  }
  async function createLink(event: FormEvent) {
    event.preventDefault();
    await action(async () => {
      const saved = await save();
      const result = await signingAPI<{
        request: SigningRequest;
        signingUrl: string;
      }>(`${path}/link`, undefined, {
        method: "POST",
        body: JSON.stringify({
          revision: saved.revision,
          signerName: signerName.trim(),
          signerEmail: signerEmail.trim(),
          expiresDays,
        }),
      });
      setRequest(result.request);
      setFields(result.request.fields);
      setLink(new URL(result.signingUrl, window.location.origin).href);
      setNotice(
        "Your signing link is ready. Copy it and send it to your customer.",
      );
    });
  }
  function leave() {
    if (!dirty || window.confirm("Leave without saving your field changes?"))
      navigate("/documents/signing");
  }

  return (
    <Page
      eyebrow="Documents / Signing"
      title={request.title}
      detail={request.fileName}
      action={<Status request={request} />}
    >
      <div className="signing-section-heading">
        <button className="back-button" onClick={leave}>
          ← Signing requests
        </button>
        <div className="button-row">
          <button
            className="secondary-button"
            disabled={busy}
            onClick={() =>
              action(async () => {
                await downloadPDF(
                  `${path}/pdf${request.status === "completed" ? "?completed=true" : ""}`,
                  request.status === "completed"
                    ? `signed-${request.fileName}`
                    : request.fileName,
                );
              })
            }
          >
            <Download size={16} />
            {request.status === "completed"
              ? "Download signed PDF"
              : request.flattened ? "Download prepared PDF" : "Download original"}
          </button>
          {request.flattened && (
            <button
              className="secondary-button"
              disabled={busy}
              onClick={() => action(async () => {
                await downloadPDF(`${path}/pdf?uploaded=true`, `uploaded-${request.fileName}`);
              })}
            >
              <Download size={16} />
              Download uploaded PDF
            </button>
          )}
          {editable && (
            <button
              className="primary-button"
              disabled={busy || !dirty}
              onClick={() =>
                action(async () => {
                  await save();
                  setNotice("Fields saved.");
                })
              }
            >
              <Save size={16} />
              {busy ? "Saving…" : "Save fields"}
            </button>
          )}
        </div>
      </div>
      {error && <ErrorState message={error} />}
      {request.flattened && (
        <section className="signing-notice" aria-label="Prepared PDF">
          <strong>Review the prepared PDF before sharing.</strong>
          <p>
            Forms, comments, and layers were flattened into a fixed copy. Review every page
            to confirm it looks right. Document text is no longer selectable. Your original
            upload is retained and available through “Download uploaded PDF”.
          </p>
        </section>
      )}
      {notice && (
        <p className="signing-notice" role="status">
          {notice}
        </p>
      )}
      <div className="signing-workbench">
        <div className="signing-document-column">
          <PageControls page={page} count={pageCount} onChange={setPage} />
          <SigningPDF
            path={`${path}/pdf${request.status === "completed" ? "?completed=true" : ""}`}
            page={page}
            {...dimensions}
            onPageCount={setPageCount}
          >
            {request.status !== "completed" &&
              fields
                .filter((field) => field.page === page)
                .map((field) => (
                  <FieldOverlay
                    key={field.id}
                    field={field}
                    editable={editable && !busy}
                    selected={field.id === selectedID}
                    pageSize={request.pages[field.page - 1]}
                    focusOnMount={field.id === focusFieldID && field.id === selectedID}
                    onSelect={() => {
                      setSelectedID(field.id);
                      setFocusFieldID("");
                    }}
                    onChange={update}
                  />
                ))}
          </SigningPDF>
        </div>
        <aside className="signing-sidebar" aria-label="Signing setup">
          {editable ? (
            <>
              <section className="signing-card">
                <p className="eyebrow">1. Place your fields</p>
                <h2>Make room for a signature.</h2>
                <p>
                  Add a field to page {page}, drag it into place, then drag a
                  corner to resize. Arrow keys adjust a focused field or corner;
                  hold Shift for bigger steps.
                </p>
                <div className="signing-field-types">
                  {(Object.keys(fieldLabels) as SigningField["type"][]).map(
                    (type) => (
                      <button
                        key={type}
                        className="secondary-button"
                        disabled={busy || fields.length >= 100}
                        onClick={() => add(type)}
                      >
                        <Plus size={15} />
                        {fieldLabels[type]}
                      </button>
                    ),
                  )}
                </div>
              </section>
              {!!fields.length && (
                <section className="signing-card">
                  <label>
                    Selected field
                    <select
                      value={selectedID}
                      onChange={(event) => {
                        setSelectedID(event.target.value);
                        const field = fields.find(
                          (item) => item.id === event.target.value,
                        );
                        if (field) setPage(field.page);
                      }}
                    >
                      <option value="">Choose a field</option>
                      {fields.map((field, index) => (
                        <option key={field.id} value={field.id}>
                          {index + 1}. {field.label} · page {field.page}
                        </option>
                      ))}
                    </select>
                  </label>
                  {selected && (
                    <fieldset
                      disabled={busy}
                      className="signing-field-settings"
                    >
                      <label>
                        Field label
                        <input
                          maxLength={80}
                          value={selected.label}
                          onChange={(event) =>
                            update({ ...selected, label: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        Field page
                        <select
                          value={selected.page}
                          onChange={(event) => {
                            const target = Number(event.target.value);
                            update({ ...selected, page: target });
                            setPage(target);
                          }}
                        >
                          {request.pages.map((_, index) => (
                            <option key={index} value={index + 1}>
                              {index + 1}
                            </option>
                          ))}
                        </select>
                      </label>
                      <details className="signing-precise-position">
                        <summary>Precise position</summary>
                        <div className="signing-position">
                          {(["x", "y", "width", "height"] as const).map((key) => (
                            <label key={key}>
                              {
                                {
                                  x: "Left (%)",
                                  y: "Top (%)",
                                  width: "Width (%)",
                                  height: "Height (%)",
                                }[key]
                              }
                              <input
                                type="number"
                                step="0.5"
                                min={
                                  key === "width" ? 5 : key === "height" ? 1.5 : 0
                                }
                                max={100}
                                value={Math.round(selected[key] * 10000) / 100}
                                onChange={(event) => {
                                  const value = event.target.valueAsNumber;
                                  if (Number.isFinite(value))
                                    update({ ...selected, [key]: value / 100 });
                                }}
                              />
                            </label>
                          ))}
                        </div>
                      </details>
                      <label className="signing-checkbox">
                        <input
                          type="checkbox"
                          checked={selected.required}
                          onChange={(event) =>
                            update({
                              ...selected,
                              required: event.target.checked,
                            })
                          }
                        />
                        Required field
                      </label>
                      <button
                        className="danger-button"
                        onClick={() => {
                          setFields((current) =>
                            current.filter((field) => field.id !== selected.id),
                          );
                          setSelectedID("");
                        }}
                      >
                        <Trash2 size={15} />
                        Remove field
                      </button>
                    </fieldset>
                  )}
                </section>
              )}
              <form className="signing-card" onSubmit={createLink}>
                <p className="eyebrow">2. Share for signing</p>
                <h2>Who is signing?</h2>
                <label>
                  Recipient name
                  <input
                    required
                    maxLength={120}
                    autoComplete="name"
                    value={signerName}
                    onChange={(event) => setSignerName(event.target.value)}
                  />
                </label>
                <label>
                  Recipient email
                  <input
                    required
                    type="email"
                    maxLength={254}
                    autoComplete="email"
                    value={signerEmail}
                    onChange={(event) => setSignerEmail(event.target.value)}
                  />
                </label>
                <label>
                  Link expires in
                  <select
                    value={expiresDays}
                    onChange={(event) =>
                      setExpiresDays(Number(event.target.value))
                    }
                  >
                    {[1, 7, 14, 30, 60, 90].map((days) => (
                      <option key={days} value={days}>
                        {days} {days === 1 ? "day" : "days"}
                      </option>
                    ))}
                  </select>
                </label>
                <p>
                  Creating the link saves and locks your fields. Anyone with the
                  link can sign, so share it only with this recipient.
                </p>
                {!fields.some(
                  (field) => field.type === "signature" && field.required,
                ) && (
                  <p className="signing-hint">
                    Add at least one required signature field to continue.
                  </p>
                )}
                <button
                  className="primary-button"
                  disabled={
                    busy ||
                    !fields.some(
                      (field) => field.type === "signature" && field.required,
                    )
                  }
                >
                  <FileSignature size={17} />
                  {busy ? "Preparing link…" : "Create signing link"}
                </button>
              </form>
            </>
          ) : (
            <section className="signing-card signing-request-summary">
              <p className="eyebrow">Signing request</p>
              <h2>
                {request.status === "completed"
                  ? "Signed, sealed, saved."
                  : request.status === "revoked"
                    ? "Link revoked"
                    : "Ready for your customer"}
              </h2>
              <p>
                {request.signerName}
                <br />
                {request.signerEmail}
              </p>
              {request.completedAt && (
                <p>
                  Completed {shortDate(request.completedAt)}. The signed PDF
                  includes the signing record.
                </p>
              )}
              {request.expiresAt && request.status === "pending" && (
                <p>Link expires {shortDate(request.expiresAt)}.</p>
              )}
              {link ? (
                <>
                  <label>
                    Private signing link
                    <input
                      readOnly
                      value={link}
                      onFocus={(event) => event.target.select()}
                    />
                  </label>
                  <button
                    className="primary-button"
                    onClick={() =>
                      action(async () => {
                        await navigator.clipboard.writeText(link);
                        setNotice("Signing link copied.");
                      })
                    }
                    disabled={busy}
                  >
                    <Copy size={16} />
                    Copy signing link
                  </button>
                  <p>
                    Copy this link before leaving. It is shown only when
                    created.
                  </p>
                </>
              ) : (
                request.status === "pending" && (
                  <p>
                    The private link was shown when created. If it is lost,
                    revoke this request and upload the PDF again.
                  </p>
                )
              )}
              {request.status === "pending" && (
                <>
                  <button
                    className="secondary-button"
                    disabled={busy}
                    onClick={() =>
                      action(async () => {
                        setRequest(await signingAPI<SigningRequest>(path));
                        setNotice("Status refreshed.");
                      })
                    }
                  >
                    Refresh status
                  </button>
                  {confirmRevoke ? (
                    <div className="signing-revoke-confirm">
                      <p>
                        Revoke this link? Your customer will no longer be able
                        to sign.
                      </p>
                      <button
                        className="danger-button"
                        disabled={busy}
                        onClick={() =>
                          action(async () => {
                            setRequest(
                              await signingAPI<SigningRequest>(
                                `${path}/revoke`,
                                undefined,
                                {
                                  method: "POST",
                                  body: JSON.stringify({
                                    revision: request.revision,
                                  }),
                                },
                              ),
                            );
                            setLink("");
                            setConfirmRevoke(false);
                            setNotice("Signing link revoked.");
                          })
                        }
                      >
                        Yes, revoke link
                      </button>
                      <button
                        className="secondary-button"
                        onClick={() => setConfirmRevoke(false)}
                      >
                        Keep link active
                      </button>
                    </div>
                  ) : (
                    <button
                      className="danger-button"
                      disabled={busy}
                      onClick={() => setConfirmRevoke(true)}
                    >
                      Revoke link
                    </button>
                  )}
                </>
              )}
            </section>
          )}
          {request.status === "completed" && request.session && (
            <SigningSessionDetails session={request.session} />
          )}
        </aside>
      </div>
    </Page>
  );
}
