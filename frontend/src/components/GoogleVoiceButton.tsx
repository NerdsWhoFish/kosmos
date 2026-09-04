import { PhoneCall } from "lucide-react";

export function GoogleVoiceButton({
  phone,
  contactId,
  mode = "message",
  className = "secondary-button",
}: {
  phone: string;
  contactId?: string;
  mode?: "call" | "message";
  className?: string;
}) {
  const query = new URLSearchParams({ phone, mode, redirect: "1" });
  if (contactId) query.set("contactId", contactId);

  return (
    <span className="voice-link-action">
      <a
        className={`${className}${phone ? "" : " disabled"}`}
        href={phone ? `/api/v1/voice/link?${query}` : undefined}
        target="_blank"
        rel="noreferrer"
        aria-disabled={!phone}
        onClick={(event) => {
          if (!phone) event.preventDefault();
        }}
      >
        <PhoneCall size={16} /> Google Voice
      </a>
    </span>
  );
}
