import { useEffect, useState } from "react";
import { ExternalLink, PhoneCall } from "lucide-react";

export function GoogleVoiceButton({
  phone,
  mode = "message",
  className = "secondary-button",
}: {
  phone: string;
  mode?: "call" | "message";
  className?: string;
}) {
  const [installed, setInstalled] = useState(false);
  const [prompt, setPrompt] = useState(false);

  useEffect(() => {
    const ready = () => setInstalled(true);
    window.addEventListener("kosmos-companion-ready", ready);
    window.postMessage(
      { type: "KOSMOS_COMPANION_PING" },
      window.location.origin,
    );
    const timer = window.setTimeout(
      () =>
        window.postMessage(
          { type: "KOSMOS_COMPANION_PING" },
          window.location.origin,
        ),
      300,
    );
    return () => {
      window.removeEventListener("kosmos-companion-ready", ready);
      window.clearTimeout(timer);
    };
  }, []);

  function openVoice() {
    if (!installed) {
      setPrompt(true);
      return;
    }
    window.postMessage(
      { type: "KOSMOS_VOICE_PREPARE", phone, mode },
      window.location.origin,
    );
  }

  return (
    <span className="voice-companion-action">
      <button
        className={className}
        type="button"
        disabled={!phone}
        onClick={openVoice}
      >
        <PhoneCall size={16} /> Google Voice
      </button>
      {prompt && (
        <span className="companion-prompt" role="status">
          Install Kosmos Companion to prepare the number in Google Voice.{" "}
          <a
            href="https://github.com/NerdsWhoFish/kosmos/tree/main/extensions/kosmos-companion"
            target="_blank"
            rel="noreferrer"
          >
            Install guide <ExternalLink size={13} />
          </a>
        </span>
      )}
    </span>
  );
}
