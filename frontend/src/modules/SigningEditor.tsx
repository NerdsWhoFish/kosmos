import { useEffect, useState, type FormEvent } from "react";
import { Copy, Download, FileSignature, Plus, Save, Trash2 } from "lucide-react";
import { Page } from "../components/Page";
import { ErrorState } from "../components/States";
import { SigningPDF } from "../components/SigningPDF";
import { shortDate } from "../api";
import { boundedField, downloadPDF, fieldLabels, signingAPI, signingError as errorMessage, type SigningField, type SigningRequest, type SigningSigner } from "./signingApi";
import { FieldOverlay, PageControls, Status } from "./SigningFields";
import { SigningSessionDetails } from "./SigningSessionDetails";
import { SigningDeadline, useSigningDeadline } from "./SigningDeadline";

const endpoint = "/api/v1/signing-requests";

export function SigningEditor({
  initial,
  navigate,
}: {
  initial: SigningRequest;
  navigate: (path: string) => void;
}) {
  const [request, setRequest] = useState(initial);
  const [signers, setSigners] = useState<SigningSigner[]>(() => initial.signers?.length ? initial.signers : [{ id: crypto.randomUUID(), name: initial.signerName ?? "", email: initial.signerEmail ?? "" }]);
  const [fields, setFields] = useState(() => (initial.fields ?? []).map((field) => initial.status === "draft" ? { ...field, signerId: field.signerId ?? signers[0].id } : field));
  const [activeSignerId, setActiveSignerId] = useState(signers[0].id);
  const [selectedID, setSelectedID] = useState("");
  const [focusFieldID, setFocusFieldID] = useState("");
  const [page, setPage] = useState(1);
  const [expiresDays, setExpiresDays] = useState(14);
  const [link, setLink] = useState("");
  const [signingLinks, setSigningLinks] = useState<{ signerId: string; signingUrl: string }[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [downloadLink, setDownloadLink] = useState("");
  const [expiresMinutes, setExpiresMinutes] = useState(60);
  const downloadDeadline = useSigningDeadline(request.downloadExpiresAt);
  const editable = request.status === "draft";
  const dirty =
    editable && (JSON.stringify(fields) !== JSON.stringify(request.fields ?? []) || JSON.stringify(signers) !== JSON.stringify(request.signers ?? []));
  const signedCount = request.signers?.filter((signer) => signer.signedAt).length ?? 0;
  const hasSignedCopy = request.status === "completed" || signedCount > 0;
  const allHaveSignature = signers.every((signer) => fields.some((field) => field.signerId === signer.id && field.type === "signature" && field.required));
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
      signerId: activeSignerId,
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
      body: JSON.stringify({ revision: request.revision, fields, signers: signers.map(({ id, name, email }) => ({ id, name: name.trim(), email: email?.trim() })) }),
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
        signingLinks?: { signerId: string; signingUrl: string }[];
      }>(`${path}/link`, undefined, {
        method: "POST",
        body: JSON.stringify({
          revision: saved.revision,
          expiresDays,
        }),
      });
      setRequest(result.request);
      setFields(result.request.fields);
      if (result.signingLinks) setSigningLinks(result.signingLinks.map((item) => ({ ...item, signingUrl: new URL(item.signingUrl, window.location.origin).href })));
      else if (result.signingUrl) setLink(new URL(result.signingUrl, window.location.origin).href);
      setNotice(
        "Your signing links are ready. Send each person their own private link. Everyone can sign immediately.",
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
                  `${path}/pdf${hasSignedCopy ? "?completed=true" : ""}`,
                  hasSignedCopy
                    ? `signed-${request.fileName}`
                    : request.fileName,
                );
              })
            }
          >
            <Download size={16} />
            {hasSignedCopy
              ? request.status === "completed" ? "Download signed PDF" : "Download current signed copy"
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
            key={hasSignedCopy ? request.signedSHA256 ?? request.updatedAt : request.id}
            path={`${path}/pdf${hasSignedCopy ? "?completed=true" : ""}`}
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
                    field={{ ...field, label: request.signers?.length || editable ? `${field.label} · ${signers.find((signer) => signer.id === field.signerId)?.name || "Unnamed signer"}` : field.label }}
                    editable={editable && !busy}
                    selected={field.id === selectedID}
                    signerIndex={signers.findIndex((signer) => signer.id === field.signerId)}
                    pageSize={request.pages[field.page - 1]}
                    focusOnMount={field.id === focusFieldID && field.id === selectedID}
                    onSelect={() => {
                      setSelectedID(field.id);
                      setFocusFieldID("");
                    }}
                    onChange={(changed) => update({ ...changed, label: field.label })}
                  />
                ))}
          </SigningPDF>
        </div>
        <aside className="signing-sidebar" aria-label="Signing setup">
          {editable ? (
            <>
              <section className="signing-card" aria-labelledby="signers-heading">
                <p className="eyebrow">1. Add your signers</p>
                <h2 id="signers-heading">Everyone gets their own link.</h2>
                <p>Add customers and staff. Everyone can sign immediately, in any order.</p>
                {signers.map((signer, index) => <fieldset key={signer.id} disabled={busy} className="signing-recipient">
                  <legend>Signer {index + 1}</legend>
                  <label>Signer {index + 1} name<input form="signing-link-form" required maxLength={120} value={signer.name} onChange={(event) => setSigners((current) => current.map((item) => item.id === signer.id ? { ...item, name: event.target.value } : item))} /></label>
                  <label>Signer {index + 1} email<input form="signing-link-form" required type="email" maxLength={254} value={signer.email ?? ""} onChange={(event) => setSigners((current) => current.map((item) => item.id === signer.id ? { ...item, email: event.target.value } : item))} /></label>
                  {signers.length > 1 && <>
                    <button className="text-button" disabled={fields.some((field) => field.signerId === signer.id)} onClick={() => {
                      setSigners((current) => current.filter((item) => item.id !== signer.id));
                      if (activeSignerId === signer.id) setActiveSignerId(signers.find((item) => item.id !== signer.id)!.id);
                    }}>Remove signer {index + 1}</button>
                    {fields.some((field) => field.signerId === signer.id) && <p className="signing-hint">Reassign or remove this person’s fields before removing them.</p>}
                  </>}
                </fieldset>)}
                <button className="secondary-button" disabled={busy || signers.length >= 10} onClick={() => {
                  const id = crypto.randomUUID();
                  setSigners((current) => [...current, { id, name: "", email: "" }]);
                  setActiveSignerId(id);
                }}><Plus size={15} />Add signer</button>
              </section>
              <section className="signing-card">
                <p className="eyebrow">2. Place your fields</p>
                <h2>Make room for a signature.</h2>
                <p>
                  Add a field to page {page}, drag it into place, then drag a
                  corner to resize. Arrow keys adjust a focused field or corner;
                  hold Shift for bigger steps.
                </p>
                <label>Place fields for<select disabled={busy} value={activeSignerId} onChange={(event) => setActiveSignerId(event.target.value)}>
                  {signers.map((signer, index) => <option key={signer.id} value={signer.id}>{signer.name || `Signer ${index + 1}`}</option>)}
                </select></label>
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
                          {index + 1}. {field.label} · {signers.find((signer) => signer.id === field.signerId)?.name || "Unnamed signer"} · page {field.page}
                        </option>
                      ))}
                    </select>
                  </label>
                  {selected && (
                    <fieldset
                      disabled={busy}
                      className="signing-field-settings"
                    >
                      <label>Assigned signer<select value={selected.signerId} onChange={(event) => update({ ...selected, signerId: event.target.value })}>
                        {signers.map((signer, index) => <option key={signer.id} value={signer.id}>{signer.name || `Signer ${index + 1}`}</option>)}
                      </select></label>
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
              <form id="signing-link-form" className="signing-card" onSubmit={createLink}>
                <p className="eyebrow">3. Share for signing</p>
                <h2>Ready for everyone?</h2>
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
                  Creating links saves and locks your signers and fields. Share each private link only with its intended recipient.
                </p>
                <p className="signing-hint">Each signer has 15 minutes to download after signing. Once everyone signs, you can create a new link to share the final PDF.</p>
                {!allHaveSignature && (
                  <p className="signing-hint">
                    Add at least one required signature field for every signer to continue.
                  </p>
                )}
                <button
                  className="primary-button"
                  disabled={
                    busy ||
                    !allHaveSignature
                  }
                >
                  <FileSignature size={17} />
                  {busy ? "Preparing links…" : signers.length > 1 ? "Create signing links" : "Create signing link"}
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
              {!!request.signers?.length ? <>
                <p className="signing-progress">{signedCount} of {request.signers.length} signed</p>
                <ul className="signing-recipient-progress">{request.signers.map((signer) => <li key={signer.id}>
                  <strong>{signer.name}</strong><span>{signer.email}</span><span>{signer.signedAt ? `Signed ${shortDate(signer.signedAt)}` : "Waiting for signature"}</span>
                  {!signer.signedAt && signingLinks.find((item) => item.signerId === signer.id) && <>
                    <label>Signing link for {signer.name}<input readOnly value={signingLinks.find((item) => item.signerId === signer.id)!.signingUrl} onFocus={(event) => event.target.select()} /></label>
                    <button className="secondary-button" disabled={busy} onClick={() => action(async () => {
                      await navigator.clipboard.writeText(signingLinks.find((item) => item.signerId === signer.id)!.signingUrl);
                      setNotice(`Signing link for ${signer.name} copied.`);
                    })}><Copy size={16} />Copy link for {signer.name}</button>
                  </>}
                </li>)}</ul>
                {request.status === "pending" && !!signingLinks.length && <p>Copy each link before leaving. Links are shown only when created.</p>}
              </> : <p>
                {request.signerName}
                <br />
                {request.signerEmail}
              </p>}
              {request.completedAt && (
                <p>
                  Completed {shortDate(request.completedAt)}. The signed PDF
                  includes the signing record.
                </p>
              )}
              {request.expiresAt && request.status === "pending" && (
                <p>Link expires {shortDate(request.expiresAt)}.</p>
              )}
              {link && request.status === "pending" ? (
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
                request.status === "pending" && !signingLinks.length && (
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
                        Revoke all signing links? No one else will be able to sign. Any signatures already collected are retained.
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
                            setSigningLinks([]);
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
                      {request.signers?.length ? "Revoke all signing links" : "Revoke link"}
                    </button>
                  )}
                </>
              )}
            </section>
          )}
          {request.status === "completed" && request.session && !request.signers?.length && (
            <SigningSessionDetails session={request.session} />
          )}
          {request.signers?.filter((signer) => signer.session).map((signer) => <div key={signer.id}>
            <h3>{signer.completedSignerName || signer.name}</h3>
            <SigningSessionDetails session={signer.session!} />
          </div>)}
          {request.status === "completed" && (
            <section className="signing-card" aria-labelledby="download-link-heading">
              <p className="eyebrow">Share a signed copy</p>
              <h2 id="download-link-heading">Create a download link.</h2>
              <p>Anyone with this link can download the signed PDF. It cannot be used to sign again. A new download link replaces your previous download link.</p>
              <label>
                Download link expires in
                <select disabled={busy} value={expiresMinutes} onChange={(event) => setExpiresMinutes(Number(event.target.value))}>
                  <option value={60}>1 hour</option>
                  <option value={1440}>1 day</option>
                  <option value={10080}>7 days</option>
                </select>
              </label>
              <button className="primary-button" disabled={busy} onClick={() => action(async () => {
                const result = await signingAPI<{ request: SigningRequest; downloadUrl: string; expiresAt: string }>(`${path}/download-link`, undefined, {
                  method: "POST",
                  body: JSON.stringify({ revision: request.revision, expiresMinutes }),
                });
                setRequest(result.request);
                setDownloadLink(new URL(result.downloadUrl, window.location.origin).href);
                setNotice("Download link created. Copy it before leaving this page.");
              })}>Create download link</button>
              {request.downloadExpiresAt && (downloadDeadline.expired
                ? <p>The previous download link has expired.</p>
                : <SigningDeadline expiresAt={request.downloadExpiresAt} />)}
              {downloadLink && !downloadDeadline.expired && <>
                <label>Private download link<input readOnly value={downloadLink} onFocus={(event) => event.target.select()} /></label>
                <button className="secondary-button" disabled={busy} onClick={() => action(async () => {
                  await navigator.clipboard.writeText(downloadLink);
                  setNotice("Download link copied.");
                })}><Copy size={16} />Copy download link</button>
              </>}
            </section>
          )}
          <section className="signing-card signing-delete-card" aria-labelledby="delete-document-heading">
            <h2 id="delete-document-heading">Delete document</h2>
            {request.status === "pending" ? <p>{request.signers?.length ? "Revoke all active signing links before deleting this document." : "Revoke the active signing link before deleting this document."}</p> : confirmDelete ? (
              <div role="group" aria-label="Confirm document deletion">
                <p>Delete “{request.title}”? This removes the document from Kosmos and invalidates all its links. Stored files are purged when retention allows. This cannot be undone.</p>
                <div className="button-row">
                  <button className="danger-button" disabled={busy} onClick={() => action(async () => {
                    await signingAPI<void>(path, undefined, {
                      method: "DELETE",
                      body: JSON.stringify({ revision: request.revision, confirmed: true }),
                    });
                    navigate("/documents/signing");
                  })}>Yes, delete document</button>
                  <button className="secondary-button" disabled={busy} onClick={() => setConfirmDelete(false)}>Keep document</button>
                </div>
              </div>
            ) : <button className="danger-button" disabled={busy} onClick={() => setConfirmDelete(true)}><Trash2 size={16} />Delete document</button>}
          </section>
        </aside>
      </div>
    </Page>
  );
}
