function openComposer(mode) {
  const pattern =
    mode === "call"
      ? /make a call|new call|call/i
      : /send a message|new message|message/i;
  const launcher = [
    ...document.querySelectorAll("button, [role='button']"),
  ].find((candidate) =>
    pattern.test(
      `${candidate.getAttribute("aria-label") || ""} ${candidate.textContent || ""}`,
    ),
  );
  launcher?.click();
}

function fillNumber(phone, mode, attempt = 0) {
  const input = [...document.querySelectorAll("input")].find((candidate) =>
    /name or number|phone number|search/i.test(
      `${candidate.getAttribute("aria-label") || ""} ${candidate.getAttribute("placeholder") || ""}`,
    ),
  );
  if (!input && attempt === 0) openComposer(mode);
  if (!input && attempt < 30) {
    setTimeout(() => fillNumber(phone, mode, attempt + 1), 300);
    return;
  }
  if (!input) return;
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, phone);
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
  input.focus();
}

chrome.runtime.onMessage.addListener((message) => {
  if (message?.type === "fill-number") fillNumber(message.phone, message.mode);
});
chrome.runtime.sendMessage({ type: "voice-ready" });
