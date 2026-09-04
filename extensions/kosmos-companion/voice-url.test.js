import test from "node:test";
import assert from "node:assert/strict";
import { safeVoiceLaunchURL } from "./voice-url.js";

test("accepts a Google account chooser that continues to Voice", () => {
  const destination =
    "https://voice.google.com/messages?authuser=shared%40example.com";
  const input = `https://accounts.google.com/AccountChooser?Email=shared%40example.com&continue=${encodeURIComponent(destination)}`;

  assert.equal(new URL(safeVoiceLaunchURL(input)).hostname, "accounts.google.com");
});

test("rejects non-Google and non-Voice destinations", () => {
  assert.equal(safeVoiceLaunchURL("https://example.com"), "");
  assert.equal(
    safeVoiceLaunchURL(
      "https://accounts.google.com/AccountChooser?continue=https%3A%2F%2Fexample.com",
    ),
    "",
  );
});
