window.addEventListener("message", (event) => {
  if (event.source !== window || event.origin !== window.location.origin)
    return;
  if (event.data?.type === "KOSMOS_COMPANION_PING")
    window.dispatchEvent(new CustomEvent("kosmos-companion-ready"));
  if (event.data?.type === "KOSMOS_VOICE_PREPARE" && event.data.phone)
    chrome.runtime.sendMessage({
      type: "prepare",
      phone: event.data.phone,
      mode: event.data.mode,
    });
});
window.dispatchEvent(new CustomEvent("kosmos-companion-ready"));
