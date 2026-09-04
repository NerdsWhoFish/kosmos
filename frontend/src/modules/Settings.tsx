import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  BookUser,
  Cloud,
  Copy,
  KeyRound,
  Mail,
  RefreshCw,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Unplug,
  UserRound,
  Users,
  Webhook,
} from "lucide-react";
import {
  Account,
  APICredential,
  APICredentialCreation,
  api,
  AuditEntry,
  CloudflareStatus,
  GoogleContactsStatus,
  GoogleStatus,
  Member,
  PipelineStage,
  SendAsMapping,
  shortDate,
  TillerProductMapping,
  TillerWebhookStatus,
  User,
} from "../api";
import { Page } from "../components/Page";
import { ErrorState, LoadingState } from "../components/States";

export function Settings({ user }: { user: User }) {
  const [members, setMembers] = useState<Member[]>([]);
  const [stages, setStages] = useState<PipelineStage[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [google, setGoogle] = useState<GoogleStatus | null>(null);
  const [googleContacts, setGoogleContacts] =
    useState<GoogleContactsStatus | null>(null);
  const [cloudflare, setCloudflare] = useState<CloudflareStatus | null>(null);
  const [sendAsMappings, setSendAsMappings] = useState<SendAsMapping[]>([]);
  const [sendAsDrafts, setSendAsDrafts] = useState<Record<string, string>>({});
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [tillerWebhook, setTillerWebhook] =
    useState<TillerWebhookStatus | null>(null);
  const [tillerMappings, setTillerMappings] = useState<TillerProductMapping[]>(
    [],
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [cloudflareError, setCloudflareError] = useState("");
  const [integrationNotice, setIntegrationNotice] = useState("");
  const [savingCloudflare, setSavingCloudflare] = useState(false);
  const [savingGoogleContacts, setSavingGoogleContacts] = useState(false);
  const [savingSendAs, setSavingSendAs] = useState("");
  const [savingTiller, setSavingTiller] = useState(false);
  const [apiCredentials, setAPICredentials] = useState<APICredential[]>([]);
  const [createdAPIToken, setCreatedAPIToken] = useState("");
  const [apiCredentialNotice, setAPICredentialNotice] = useState("");
  const [savingAPICredential, setSavingAPICredential] = useState(false);
  const [revokingCredentialID, setRevokingCredentialID] = useState("");

  const currentMember = members.find(
    (member) => member.email.toLowerCase() === user.email.toLowerCase(),
  );
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";

  const load = useCallback(() => {
    Promise.all([
      api<{ members: Member[] }>("/api/v1/members"),
      api<{ stages: PipelineStage[] }>("/api/v1/pipeline-stages"),
      api<{ entries: AuditEntry[] }>("/api/v1/audit"),
      api<GoogleStatus>("/api/v1/integrations/google"),
      api<CloudflareStatus>("/api/v1/integrations/cloudflare"),
      api<{ accounts: Account[] }>("/api/v1/accounts"),
    ])
      .then(
        ([
          team,
          pipeline,
          history,
          connection,
          cloudflareConnection,
          accountResponse,
        ]) => {
          setMembers(team.members);
          setStages(pipeline.stages);
          setAudit(history.entries);
          setGoogle(connection);
          setCloudflare(cloudflareConnection);
          setAccounts(accountResponse.accounts);
          const current = team.members.find(
            (member) => member.email.toLowerCase() === user.email.toLowerCase(),
          );
          if (current?.role === "owner" || current?.role === "admin") {
            void api<{ mappings: SendAsMapping[] }>("/api/v1/email/send-as")
              .then((result) => setSendAsMappings(result.mappings ?? []))
              .catch((reason: Error) => setError(reason.message));
            void Promise.all([
              api<GoogleContactsStatus>(
                "/api/v1/integrations/google-contacts",
              ),
              api<TillerWebhookStatus>("/api/v1/integrations/tiller/webhook"),
              api<{ mappings: TillerProductMapping[] }>(
                "/api/v1/integrations/tiller/product-mappings",
              ),
              api<{ credentials: APICredential[] }>(
                "/api/v1/api-credentials",
              ),
            ])
              .then(([contactsStatus, status, mappings, credentials]) => {
                setGoogleContacts(contactsStatus);
                setTillerWebhook(status);
                setTillerMappings(mappings.mappings ?? []);
                setAPICredentials(credentials.credentials ?? []);
              })
              .catch((reason: Error) => setError(reason.message));
          }
          setError("");
        },
      )
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, [user.email]);
  useEffect(load, [load]);

  async function changeRole(member: Member, role: Member["role"]) {
    try {
      await api(`/api/v1/members/${member.id}`, {
        method: "PATCH",
        body: JSON.stringify({ role, status: member.status }),
      });
      load();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not update role",
      );
    }
  }

  async function createAPICredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    setSavingAPICredential(true);
    setAPICredentialNotice("");
    setCreatedAPIToken("");
    try {
      const result = await api<APICredentialCreation>(
        "/api/v1/api-credentials",
        {
          method: "POST",
          body: JSON.stringify({
            name: data.get("name"),
            access: data.get("access"),
          }),
        },
      );
      setAPICredentials((current) => [result.credential, ...current]);
      setCreatedAPIToken(result.token);
      setAPICredentialNotice(
        "Copy this token now. Kosmos cannot show it again.",
      );
      form.reset();
    } catch (reason) {
      setAPICredentialNotice(
        reason instanceof Error
          ? reason.message
          : "Could not create API credential",
      );
    } finally {
      setSavingAPICredential(false);
    }
  }

  async function copyAPIToken() {
    await navigator.clipboard.writeText(createdAPIToken);
    setAPICredentialNotice("API token copied. Store it somewhere secure.");
  }

  async function revokeAPICredential(credential: APICredential) {
    try {
      await api(`/api/v1/api-credentials/${credential.id}`, {
        method: "DELETE",
      });
      const revokedAt = new Date().toISOString();
      setAPICredentials((current) =>
        current.map((item) =>
          item.id === credential.id ? { ...item, revokedAt } : item,
        ),
      );
      setRevokingCredentialID("");
      setAPICredentialNotice(`${credential.name} was revoked.`);
    } catch (reason) {
      setAPICredentialNotice(
        reason instanceof Error
          ? reason.message
          : "Could not revoke API credential",
      );
    }
  }

  async function createStage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await api("/api/v1/pipeline-stages", {
        method: "POST",
        body: JSON.stringify({
          name: form.get("name"),
          position: Number(form.get("position")),
          probability: Number(form.get("probability")),
          closed: form.get("closed") === "on",
          won: form.get("won") === "on",
        }),
      });
      formElement.reset();
      load();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not create stage",
      );
    }
  }

  async function connectCloudflare(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSavingCloudflare(true);
    setCloudflareError("");
    setIntegrationNotice("");
    const form = new FormData(formElement);
    try {
      const connection = await api<CloudflareStatus>(
        "/api/v1/integrations/cloudflare",
        {
          method: "PUT",
          body: JSON.stringify({
            accountId: form.get("accountId"),
            apiToken: form.get("apiToken"),
          }),
        },
      );
      setCloudflare(connection);
      setIntegrationNotice(
        `Cloudflare connected. ${connection.domainCount ?? 0} domains are ready to link from Accounts.`,
      );
      formElement.reset();
    } catch (reason) {
      setCloudflareError(
        reason instanceof Error
          ? reason.message
          : "Could not connect Cloudflare",
      );
    } finally {
      setSavingCloudflare(false);
    }
  }

  async function disconnectCloudflare() {
    setSavingCloudflare(true);
    setCloudflareError("");
    try {
      await api("/api/v1/integrations/cloudflare", { method: "DELETE" });
      setCloudflare({ connected: false });
      setIntegrationNotice(
        "Cloudflare disconnected. Existing account domains and reminders were kept.",
      );
    } catch (reason) {
      setCloudflareError(
        reason instanceof Error
          ? reason.message
          : "Could not disconnect Cloudflare",
      );
    } finally {
      setSavingCloudflare(false);
    }
  }

  async function syncGoogleContacts() {
    setSavingGoogleContacts(true);
    setIntegrationNotice("");
    try {
      const result = await api<{ queued: number }>(
        "/api/v1/integrations/google-contacts/sync",
        { method: "POST" },
      );
      const status = await api<GoogleContactsStatus>(
        "/api/v1/integrations/google-contacts",
      );
      setGoogleContacts(status);
      setIntegrationNotice(
        `${result.queued} Kosmos contact${result.queued === 1 ? "" : "s"} queued for Google Contacts sync.`,
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not synchronize Google Contacts",
      );
    } finally {
      setSavingGoogleContacts(false);
    }
  }

  async function disconnectGoogleContacts() {
    setSavingGoogleContacts(true);
    setIntegrationNotice("");
    try {
      await api("/api/v1/integrations/google-contacts", { method: "DELETE" });
      setGoogleContacts((current) => ({
        connected: false,
        googleEmail: "",
        connectUrl:
          current?.connectUrl ?? "/auth/connect/voice-contacts",
        pending: 0,
        failed: current?.failed ?? 0,
        synced: current?.synced ?? 0,
      }));
      setIntegrationNotice(
        "Shared Google Contacts disconnected. Existing contacts in Google were kept.",
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not disconnect Google Contacts",
      );
    } finally {
      setSavingGoogleContacts(false);
    }
  }

  async function saveSendAs(member: Member) {
    const email = (
      sendAsDrafts[member.id] ??
      sendAsMappings.find((mapping) => mapping.memberId === member.id)?.email ??
      member.email
    ).trim();
    setSavingSendAs(member.id);
    setIntegrationNotice("");
    try {
      const mapping = await api<SendAsMapping>(
        `/api/v1/members/${member.id}/send-as`,
        { method: "PUT", body: JSON.stringify({ email }) },
      );
      setSendAsMappings((current) => [
        ...current.filter((item) => item.memberId !== member.id),
        mapping,
      ]);
      setIntegrationNotice(
        `${member.name} will send Kosmos email from ${mapping.email}.`,
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not save sender address",
      );
    } finally {
      setSavingSendAs("");
    }
  }

  async function connectTillerWebhook(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSavingTiller(true);
    setIntegrationNotice("");
    try {
      const status = await api<TillerWebhookStatus>(
        "/api/v1/integrations/tiller/webhook",
        {
          method: "PUT",
          body: JSON.stringify({
            signingSecret: new FormData(formElement).get("signingSecret"),
          }),
        },
      );
      setTillerWebhook(status);
      setIntegrationNotice("Tiller purchase webhook connected.");
      formElement.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not connect Tiller webhook",
      );
    } finally {
      setSavingTiller(false);
    }
  }

  async function saveTillerMapping(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const productId = String(form.get("productId")).trim();
    setSavingTiller(true);
    try {
      const mapping = await api<TillerProductMapping>(
        `/api/v1/integrations/tiller/product-mappings/${encodeURIComponent(productId)}`,
        {
          method: "PUT",
          body: JSON.stringify({
            productName: form.get("productName"),
            accountId: form.get("accountId"),
          }),
        },
      );
      setTillerMappings((current) => [
        ...current.filter((item) => item.productId !== mapping.productId),
        mapping,
      ]);
      setIntegrationNotice(
        `Tiller product ${mapping.productId} will record purchases against its mapped account.`,
      );
      formElement.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not save Tiller product mapping",
      );
    } finally {
      setSavingTiller(false);
    }
  }

  async function removeTillerMapping(mapping: TillerProductMapping) {
    await api(
      `/api/v1/integrations/tiller/product-mappings/${encodeURIComponent(mapping.productId)}`,
      { method: "DELETE" },
    );
    setTillerMappings((current) =>
      current.filter((item) => item.id !== mapping.id),
    );
  }

  if (loading) return <LoadingState />;
  if (error) return <ErrorState message={error} retry={load} />;

  return (
    <Page
      eyebrow="Workspace"
      title="Settings"
      detail="People, permissions, integrations, and the audit trail without an admin maze."
    >
      <div className="settings-grid">
        <section className="panel setting-card">
          <span className="setting-icon">
            <UserRound size={20} />
          </span>
          <div>
            <h2>Your Google account</h2>
            <p>{user.name}</p>
            <small>{user.email}</small>
          </div>
        </section>
        <section className="panel setting-card">
          <span className="setting-icon">
            <ShieldCheck size={20} />
          </span>
          <div>
            <h2>Access policy</h2>
            <p>Approved domains only</p>
            <small>
              Every request rechecks the verified Google identity and active
              membership.
            </small>
          </div>
        </section>
        <section className="panel setting-card">
          <span className="setting-icon">
            <KeyRound size={20} />
          </span>
          <div>
            <h2>Password and MFA</h2>
            <p>Managed by Google</p>
            <small>Kosmos never stores or resets your password.</small>
          </div>
        </section>
      </div>
      {integrationNotice && (
        <p className="success-notice" role="status">
          {integrationNotice}
        </p>
      )}
      <section className="split-grid lower-grid">
        <div className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Team</p>
              <h2>Members and roles</h2>
            </div>
            <Users size={20} />
          </div>
          <div className="table-list">
            {members.map((member) => (
              <article className="member-row" key={member.id}>
                <span className="record-avatar">
                  {member.name
                    .split(" ")
                    .map((part) => part[0])
                    .join("")
                    .slice(0, 2)}
                </span>
                <span>
                  <strong>{member.name}</strong>
                  <small>{member.email}</small>
                </span>
                <select
                  aria-label={`Role for ${member.name}`}
                  value={member.role}
                  disabled={!canManage}
                  onChange={(event) =>
                    changeRole(member, event.target.value as Member["role"])
                  }
                >
                  <option value="owner">Owner</option>
                  <option value="admin">Admin</option>
                  <option value="member">Member</option>
                  <option value="viewer">Viewer</option>
                </select>
              </article>
            ))}
          </div>
        </div>
        <div className="panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Google Workspace</p>
              <h2>{google?.connected ? "Connected" : "Not connected"}</h2>
            </div>
            <SlidersHorizontal size={20} />
          </div>
          <p className="muted-copy">
            Gmail compose, relevant message metadata, verified send-as aliases,
            and read-only Google Sheets access are granted separately from
            login.
          </p>
          {google?.connected ? (
            <>
              <p className="inline-notice">
                <span className="security-dot" />{" "}
                {google.connection?.googleEmail}
              </p>
              <a className="text-button" href={google.connectUrl}>
                Reconnect Google permissions
              </a>
            </>
          ) : (
            <a
              className="primary-button"
              href={google?.connectUrl ?? "/auth/connect/workspace"}
            >
              Connect Google Workspace
            </a>
          )}
        </div>
      </section>
      {canManage && (
        <section className="panel lower-panel api-credentials-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Workflow access</p>
              <h2>API credentials</h2>
            </div>
            <KeyRound size={20} />
          </div>
          <p className="muted-copy">
            Create a named token for an external workflow. Read-only tokens can
            inspect ordinary workspace records. Read-and-write tokens can also
            change them, but no token can manage people, credentials, email, or
            integrations.
          </p>
          <form
            className="api-credential-form"
            onSubmit={createAPICredential}
          >
            <label>
              Credential name
              <input
                name="name"
                maxLength={80}
                placeholder="Brand guide publisher"
                required
              />
            </label>
            <label>
              Access
              <select name="access" defaultValue="read">
                <option value="read">Read only</option>
                <option value="write">Read and write</option>
              </select>
            </label>
            <button className="primary-button" disabled={savingAPICredential}>
              <KeyRound size={16} />
              {savingAPICredential ? "Creating..." : "Create credential"}
            </button>
          </form>
          {createdAPIToken && (
            <div className="api-token-reveal">
              <strong>Copy this token now</strong>
              <code>{createdAPIToken}</code>
              <button
                className="secondary-button"
                type="button"
                onClick={copyAPIToken}
              >
                <Copy size={16} /> Copy token
              </button>
            </div>
          )}
          {apiCredentialNotice && (
            <p className="inline-notice" role="status">
              {apiCredentialNotice}
            </p>
          )}
          <div className="api-credential-list">
            {apiCredentials.length ? (
              apiCredentials.map((credential) => (
                <article className="api-credential-row" key={credential.id}>
                  <span>
                    <strong>{credential.name}</strong>
                    <small>
                      {credential.access === "write"
                        ? "Read and write"
                        : "Read only"}{" · "}
                      {credential.tokenPrefix}
                    </small>
                    <small>
                      Created {shortDate(credential.createdAt)} by{" "}
                      {credential.createdBy}
                    </small>
                  </span>
                  {credential.revokedAt ? (
                    <span className="status-pill muted">Revoked</span>
                  ) : revokingCredentialID === credential.id ? (
                    <span className="inline-confirm">
                      <strong>Revoke access?</strong>
                      <button
                        className="text-button"
                        type="button"
                        onClick={() => setRevokingCredentialID("")}
                      >
                        Keep
                      </button>
                      <button
                        className="text-button danger-text"
                        type="button"
                        onClick={() => revokeAPICredential(credential)}
                      >
                        Revoke
                      </button>
                    </span>
                  ) : (
                    <button
                      className="icon-button danger-icon"
                      type="button"
                      aria-label={`Revoke ${credential.name}`}
                      onClick={() => setRevokingCredentialID(credential.id)}
                    >
                      <Trash2 size={16} />
                    </button>
                  )}
                </article>
              ))
            ) : (
              <p className="muted-copy">No API credentials yet.</p>
            )}
          </div>
        </section>
      )}
      {canManage && (
        <section className="panel lower-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Gmail send-as</p>
              <h2>Sender addresses</h2>
            </div>
            <Mail size={20} />
          </div>
          <p className="muted-copy">
            Map each teammate to a verified alias already configured in their
            Gmail account. Google verifies the address before Kosmos saves it.
          </p>
          <div className="send-as-list">
            {members.map((member) => {
              const configured =
                sendAsMappings.find((mapping) => mapping.memberId === member.id)
                  ?.email ?? member.email;
              return (
                <article className="send-as-row" key={member.id}>
                  <span>
                    <strong>{member.name}</strong>
                    <small>Google login: {member.email}</small>
                  </span>
                  <label>
                    Send from
                    <input
                      type="email"
                      aria-label={`Send from for ${member.name}`}
                      value={sendAsDrafts[member.id] ?? configured}
                      onChange={(event) =>
                        setSendAsDrafts((current) => ({
                          ...current,
                          [member.id]: event.target.value,
                        }))
                      }
                    />
                  </label>
                  <button
                    className="secondary-button"
                    disabled={savingSendAs === member.id}
                    onClick={() => saveSendAs(member)}
                  >
                    {savingSendAs === member.id
                      ? "Verifying..."
                      : "Verify and save"}
                  </button>
                </article>
              );
            })}
          </div>
        </section>
      )}
      {canManage && (
        <section className="panel lower-panel integration-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Google Voice contacts</p>
              <h2>
                {googleContacts?.connected
                  ? "Shared contacts connected"
                  : "Connect the shared account"}
              </h2>
            </div>
            <BookUser size={20} />
          </div>
          <p className="muted-copy">
            Connect the separate Google account your team shares for Google
            Voice. Kosmos will keep that account&apos;s Google Contacts in sync
            when contacts are created, edited, or deleted.
          </p>
          {googleContacts?.connected ? (
            <div className="integration-connected">
              <p className="inline-notice">
                <span className="security-dot" /> {googleContacts.googleEmail}
                <small>
                  {googleContacts.synced} synced · {googleContacts.pending}{" "}
                  pending · {googleContacts.failed} failed
                </small>
              </p>
              <div className="button-row">
                <button
                  className="secondary-button"
                  type="button"
                  disabled={savingGoogleContacts}
                  onClick={syncGoogleContacts}
                >
                  <RefreshCw size={16} />
                  {savingGoogleContacts ? "Queueing..." : "Sync now"}
                </button>
                <a className="text-button" href={googleContacts.connectUrl}>
                  Reconnect account
                </a>
                <button
                  className="text-button"
                  type="button"
                  disabled={savingGoogleContacts}
                  onClick={disconnectGoogleContacts}
                >
                  Disconnect
                </button>
              </div>
            </div>
          ) : (
            <a
              className="primary-button"
              href={
                googleContacts?.connectUrl ?? "/auth/connect/voice-contacts"
              }
            >
              <BookUser size={16} /> Connect shared Google account
            </a>
          )}
        </section>
      )}
      <section className="panel lower-panel integration-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Cloudflare</p>
            <h2>
              {cloudflare?.connected
                ? "Domain inventory connected"
                : "Connect your domains"}
            </h2>
          </div>
          <Cloud size={20} />
        </div>
        <p className="muted-copy">
          Kosmos reads zones and Registrar renewal dates. It cannot change DNS
          or domain registration settings.
        </p>
        {cloudflare?.connected ? (
          <div className="integration-connected">
            <p className="inline-notice">
              <span className="security-dot" /> Account {cloudflare.accountId}
            </p>
            <button
              className="secondary-button"
              type="button"
              disabled={savingCloudflare}
              onClick={disconnectCloudflare}
            >
              Disconnect Cloudflare
            </button>
          </div>
        ) : (
          <form
            className="inline-form cloudflare-form"
            onSubmit={connectCloudflare}
          >
            <label>
              Cloudflare account ID
              <input
                name="accountId"
                minLength={32}
                maxLength={32}
                required
                autoComplete="off"
              />
            </label>
            <label>
              Dedicated API token
              <input
                name="apiToken"
                type="password"
                minLength={20}
                required
                autoComplete="new-password"
              />
            </label>
            <button className="primary-button" disabled={savingCloudflare}>
              {savingCloudflare ? "Checking access..." : "Connect Cloudflare"}
            </button>
            <p className="field-help">
              Use a dedicated token with Zone Read and Registrar access. Kosmos
              encrypts it and never returns it.
            </p>
          </form>
        )}
        {cloudflareError && (
          <p className="form-error" role="alert">
            {cloudflareError}
          </p>
        )}
      </section>
      {canManage && (
        <section className="panel lower-panel integration-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">Tiller purchases</p>
              <h2>
                {tillerWebhook?.connected
                  ? "Purchase webhook connected"
                  : "Connect direct purchase events"}
              </h2>
            </div>
            <Webhook size={20} />
          </div>
          <p className="muted-copy">
            Create an <strong>order.paid</strong> webhook in Tiller for{" "}
            <code>
              {window.location.origin}
              {tillerWebhook?.endpoint ?? "/api/v1/webhooks/tiller"}
            </code>
            , then paste its one-time signing secret here. Spreadsheet import
            remains optional.
          </p>
          {!tillerWebhook?.connected && (
            <form className="integration-form" onSubmit={connectTillerWebhook}>
              <label>
                Tiller signing secret
                <input
                  type="password"
                  name="signingSecret"
                  required
                  pattern="whsec_[a-fA-F0-9]{64}"
                  autoComplete="new-password"
                  placeholder="whsec_…"
                />
              </label>
              <button className="primary-button" disabled={savingTiller}>
                {savingTiller ? "Saving..." : "Connect webhook"}
              </button>
            </form>
          )}
          <form className="tiller-mapping-form" onSubmit={saveTillerMapping}>
            <label>
              Tiller product ID
              <input name="productId" required placeholder="prod_…" />
            </label>
            <label>
              Product name
              <input
                name="productName"
                maxLength={160}
                placeholder="Optional friendly name"
              />
            </label>
            <label>
              Kosmos account
              <select name="accountId" required defaultValue="">
                <option value="" disabled>
                  Choose an account
                </option>
                {accounts.map((account) => (
                  <option value={account.id} key={account.id}>
                    {account.name}
                  </option>
                ))}
              </select>
            </label>
            <button className="secondary-button" disabled={savingTiller}>
              Save mapping
            </button>
          </form>
          {tillerMappings.length ? (
            <div className="table-list">
              {tillerMappings.map((mapping) => (
                <article className="mapping-row" key={mapping.id}>
                  <span>
                    <strong>{mapping.productName || mapping.productId}</strong>
                    <small>
                      {mapping.productId} ·{" "}
                      {accounts.find(
                        (account) => account.id === mapping.accountId,
                      )?.name || "Unknown account"}
                    </small>
                  </span>
                  <button
                    className="icon-button"
                    aria-label={`Remove ${mapping.productName || mapping.productId}`}
                    onClick={() => removeTillerMapping(mapping)}
                  >
                    <Unplug size={16} />
                  </button>
                </article>
              ))}
            </div>
          ) : (
            <p className="muted-copy">
              No products mapped yet. Unmapped purchases are safely ignored.
            </p>
          )}
        </section>
      )}
      <section className="panel lower-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Pipeline</p>
            <h2>Your stages</h2>
          </div>
        </div>
        <div className="stage-strip">
          {stages.map((stage) => (
            <span key={stage.id}>
              <strong>{stage.name}</strong>
              <small>{stage.probability}%</small>
            </span>
          ))}
        </div>
        <form className="inline-form" onSubmit={createStage}>
          <label>
            Stage name
            <input name="name" maxLength={80} required />
          </label>
          <label>
            Position
            <input
              name="position"
              type="number"
              min="0"
              defaultValue={stages.length}
              required
            />
          </label>
          <label>
            Probability
            <input
              name="probability"
              type="number"
              min="0"
              max="100"
              defaultValue="50"
              required
            />
          </label>
          <label className="check-label">
            <input name="closed" type="checkbox" /> Closed
          </label>
          <label className="check-label">
            <input name="won" type="checkbox" /> Won
          </label>
          <button className="secondary-button">Add stage</button>
        </form>
      </section>
      <section className="panel lower-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Audit history</p>
            <h2>Who changed what</h2>
          </div>
        </div>
        <div className="table-list">
          {audit.length ? (
            audit.slice(0, 30).map((entry) => (
              <article className="audit-row" key={entry.id}>
                <span>
                  <strong>{entry.summary}</strong>
                  <small>
                    {entry.actor} · {entry.action}
                  </small>
                </span>
                <time>{shortDate(entry.createdAt)}</time>
              </article>
            ))
          ) : (
            <p className="muted-copy">
              Security-sensitive and external actions will appear here.
            </p>
          )}
        </div>
      </section>
    </Page>
  );
}
