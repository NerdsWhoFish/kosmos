import { safeVoiceLaunchURL } from "./voice-url.js";

chrome.runtime.onMessage.addListener((message, sender) => {
  if (message?.type === "prepare") {
    const launchUrl = safeVoiceLaunchURL(message.launchUrl);
    if (!launchUrl) return;
    chrome.storage.session.set({
      pendingVoice: { phone: message.phone, mode: message.mode },
    });
    chrome.tabs.create({ url: launchUrl });
  }
  if (message?.type === "voice-ready") {
    chrome.storage.session.get("pendingVoice").then(({ pendingVoice }) => {
      if (!pendingVoice || !sender.tab?.id) return;
      chrome.tabs.sendMessage(sender.tab.id, {
        type: "fill-number",
        ...pendingVoice,
      });
      chrome.storage.session.remove("pendingVoice");
    });
  }
});
