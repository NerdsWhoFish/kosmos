export function safeVoiceLaunchURL(value) {
  if (typeof value !== "string") return "";
  try {
    const url = new URL(value);
    if (
      url.protocol !== "https:" ||
      url.hostname !== "accounts.google.com" ||
      url.pathname !== "/AccountChooser"
    )
      return "";
    const destination = new URL(url.searchParams.get("continue") || "");
    if (
      destination.protocol !== "https:" ||
      destination.hostname !== "voice.google.com" ||
      !["/calls", "/messages"].includes(destination.pathname)
    )
      return "";
    return url.toString();
  } catch {
    return "";
  }
}
