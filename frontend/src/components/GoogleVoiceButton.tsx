import { PhoneCall } from "lucide-react";

export function GoogleVoiceButton({
  phone,
  mode = "message",
  className = "secondary-button",
}: {
  phone: string;
  mode?: "call" | "message";
  className?: string;
}) {
  const query = new URLSearchParams({ phone, mode, redirect: "1" });

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
