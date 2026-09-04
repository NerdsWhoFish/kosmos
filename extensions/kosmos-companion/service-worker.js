chrome.runtime.onMessage.addListener((message, sender) => {
  if (message?.type === "prepare") {
    chrome.storage.session.set({
      pendingVoice: { phone: message.phone, mode: message.mode },
    });
    chrome.tabs.create({
      url: `https://voice.google.com/u/0/${message.mode === "call" ? "calls" : "messages"}`,
    });
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
