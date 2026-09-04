import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  Cloud,
  KeyRound,
  Mail,
  ShieldCheck,
  SlidersHorizontal,
  Unplug,
  UserRound,
  Users,
  Webhook,
} from "lucide-react";
import {
  Account,
  api,
  AuditEntry,
  CloudflareStatus,
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
  const [savingSendAs, setSavingSendAs] = useState("");
  const [savingTiller, setSavingTiller] = useState(false);

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
              api<TillerWebhookStatus>("/api/v1/integrations/tiller/webhook"),
              api<{ mappings: TillerProductMapping[] }>(
                "/api/v1/integrations/tiller/product-mappings",
              ),
            ])
              .then(([status, mappings]) => {
                setTillerWebhook(status);
                setTillerMappings(mappings.mappings ?? []);
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
