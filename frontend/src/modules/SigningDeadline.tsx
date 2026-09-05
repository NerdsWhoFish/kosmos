import { useEffect, useState } from "react";

export function useSigningDeadline(expiresAt?: string) {
  const [now, setNow] = useState(Date.now);
  const deadline = expiresAt ? new Date(expiresAt).getTime() : undefined;
  useEffect(() => {
    const update = () => setNow(Date.now());
    update();
    if (deadline === undefined) return;
    const timer = window.setInterval(update, 1000);
    window.addEventListener("focus", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, [deadline]);
  const seconds = deadline === undefined ? undefined : Math.max(0, Math.ceil((deadline - now) / 1000));
  return {
    expired: seconds === 0,
    remaining: seconds === undefined ? "" : seconds >= 3600
      ? `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m remaining`
      : `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")} remaining`,
  };
}

export function SigningDeadline({ expiresAt, remaining }: { expiresAt: string; remaining?: string }) {
  return (
    <p className="signing-download-deadline">
      Download available until{" "}
      <time dateTime={expiresAt}>{new Date(expiresAt).toLocaleString(undefined, {
        dateStyle: "medium", timeStyle: "long",
      })}</time>.
      {remaining && <strong role="timer" aria-label="Download time remaining">{remaining}</strong>}
    </p>
  );
}
