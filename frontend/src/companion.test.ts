import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const extensionRoot = resolve(process.cwd(), "../extensions/kosmos-companion");

describe("Kosmos Companion extension", () => {
  it("ships a Manifest V3 extension for Kosmos and Google Voice", () => {
    const manifest = JSON.parse(
      readFileSync(resolve(extensionRoot, "manifest.json"), "utf8"),
    ) as {
      manifest_version: number;
      background: { service_worker: string };
      content_scripts: Array<{ matches: string[]; js: string[] }>;
    };

    expect(manifest.manifest_version).toBe(3);
    expect(manifest.background.service_worker).toBe("service-worker.js");
    expect(manifest.content_scripts).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          matches: expect.arrayContaining([
            "https://kosmos.nerdswhofish.com/*",
          ]),
        }),
        expect.objectContaining({
          matches: expect.arrayContaining(["https://voice.google.com/*"]),
        }),
      ]),
    );
  });

  it("prepares numbers without automatically placing calls or sending texts", () => {
    const worker = readFileSync(
      resolve(extensionRoot, "service-worker.js"),
      "utf8",
    );
    const voice = readFileSync(resolve(extensionRoot, "voice.js"), "utf8");
    const readme = readFileSync(resolve(extensionRoot, "README.md"), "utf8");

    expect(worker).toContain("chrome.storage.session");
    expect(worker).toContain("chrome.tabs.create");
    expect(voice).toContain('new Event("input"');
    expect(voice).not.toMatch(/\.click\(\).*send|send.*\.click\(\)/i);
    expect(readme).toContain("safari-web-extension-converter");
  });
});
