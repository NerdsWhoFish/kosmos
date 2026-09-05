import type { SigningSession } from "./signingApi";

export function SigningSessionDetails({ session }: { session: SigningSession }) {
  const location = [session.city, session.region, session.country]
    .filter((part) => part?.trim())
    .join(", ");
  const capturedAt = new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "long",
  }).format(new Date(session.capturedAt));

  return (
    <section className="signing-card signing-session" aria-labelledby="signing-session-title">
      <p className="eyebrow">Signing record</p>
      <h2 id="signing-session-title">Signing session</h2>
      <dl>
        <div><dt>IP address</dt><dd>{session.ipAddress || "Unknown"}</dd></div>
        <div><dt>Approximate location</dt><dd>{location || "Unknown"}</dd></div>
        <div><dt>Captured</dt><dd><time dateTime={session.capturedAt}>{capturedAt}</time></dd></div>
      </dl>
      <details>
        <summary>Browser-reported details</summary>
        <p className="signing-session-browser">{session.userAgent || "Unknown"}</p>
      </details>
      <p className="signing-hint">
        Location is approximate and browser details are self-reported. This record is not proof of identity.
      </p>
    </section>
  );
}
