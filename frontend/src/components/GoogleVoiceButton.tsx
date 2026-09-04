import { useEffect, useState } from "react";
import { ExternalLink, PhoneCall } from "lucide-react";
import { api } from "../api";

type VoiceLink = {
  googleVoiceUrl: string;
  googleAccount: string;
};

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
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState("");

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

  async function openVoice() {
    if (!installed) {
      setPrompt(true);
      return;
    }
    setOpening(true);
    setError("");
    try {
      const query = new URLSearchParams({ phone, mode });
      const link = await api<VoiceLink>(`/api/v1/voice/link?${query}`);
      window.postMessage(
        {
          type: "KOSMOS_VOICE_PREPARE",
          phone,
          mode,
          launchUrl: link.googleVoiceUrl,
        },
        window.location.origin,
      );
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not open Google Voice",
      );
    } finally {
      setOpening(false);
    }
  }

  return (
    <span className="voice-companion-action">
      <button
        className={className}
        type="button"
        disabled={!phone || opening}
        onClick={openVoice}
      >
        <PhoneCall size={16} /> {opening ? "Opening..." : "Google Voice"}
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
      {error && (
        <span className="companion-prompt" role="alert">
          {error}
        </span>
      )}
    </span>
  );
}
