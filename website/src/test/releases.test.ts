import { beforeEach, describe, expect, it } from "vitest";
import { site } from "@/config/site";
import {
  fallbackReleaseAssets,
  fetchLatestRelease,
  parseGitHubRelease,
  resetReleaseCache,
} from "@/integrations/releases";

describe("releases loader", () => {
  beforeEach(() => {
    resetReleaseCache();
  });

  it("parses all platform release assets and checksums", () => {
    const parsed = parseGitHubRelease({
      tag_name: "v1.1.0",
      html_url: "https://github.com/Blinks44/hearth/releases/tag/v1.1.0",
      assets: [
        { name: "hearth-1.1.0-arm64.dmg", browser_download_url: "https://dl/hearth-arm64.dmg" },
        { name: "hearth-1.1.0-amd64.dmg", browser_download_url: "https://dl/hearth-amd64.dmg" },
        { name: "hearth-setup-1.1.0-amd64.exe", browser_download_url: "https://dl/win-x64.exe" },
        { name: "hearth-setup-1.1.0-arm64.exe", browser_download_url: "https://dl/win-arm64.exe" },
        { name: "hearth_1.1.0_amd64.deb", browser_download_url: "https://dl/linux-x64.deb" },
        { name: "hearth_1.1.0_arm64.deb", browser_download_url: "https://dl/linux-arm64.deb" },
        {
          name: "hearth-1.1.0-windows-amd64.tar.gz",
          browser_download_url: "https://dl/win-x64.tar.gz",
        },
        {
          name: "hearth-1.1.0-windows-arm64.tar.gz",
          browser_download_url: "https://dl/win-arm64.tar.gz",
        },
        {
          name: "hearth-1.1.0-linux-amd64.tar.gz",
          browser_download_url: "https://dl/linux-x64.tar.gz",
        },
        {
          name: "hearth-1.1.0-linux-arm64.tar.gz",
          browser_download_url: "https://dl/linux-arm64.tar.gz",
        },
        { name: "SHA256SUMS.txt", browser_download_url: "https://dl/SHA256SUMS.txt" },
      ],
    });

    expect(parsed.version).toBe("1.1.0");
    expect(parsed.isFallback).toBe(false);
    expect(parsed.macosArm64).toBe("https://dl/hearth-arm64.dmg");
    expect(parsed.macosAmd64).toBe("https://dl/hearth-amd64.dmg");
    expect(parsed.windowsAmd64).toBe("https://dl/win-x64.exe");
    expect(parsed.windowsArm64).toBe("https://dl/win-arm64.exe");
    expect(parsed.linuxDebAmd64).toBe("https://dl/linux-x64.deb");
    expect(parsed.linuxDebArm64).toBe("https://dl/linux-arm64.deb");
    expect(parsed.linuxTarAmd64).toBe("https://dl/linux-x64.tar.gz");
    expect(parsed.linuxTarArm64).toBe("https://dl/linux-arm64.tar.gz");
    expect(parsed.checksumsUrl).toBe("https://dl/SHA256SUMS.txt");
  });

  it("resolves latest release directly from /releases/latest redirect without hitting GitHub API", async () => {
    let calls = 0;
    const fakeFetch = async (input: string | URL | Request) => {
      calls++;
      expect(String(input)).toBe(site.latestRelease);
      return new Response(null, {
        status: 302,
        headers: { Location: "https://github.com/Blinks44/hearth/releases/tag/v1.1.0" },
      });
    };

    const first = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000);
    const second = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000 + 60_000);

    expect(calls).toBe(1);
    expect(first.version).toBe("1.1.0");
    expect(first.isFallback).toBe(false);
    expect(first.macosArm64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.1.0/hearth-1.1.0-arm64.dmg",
    );
    expect(first.macosAmd64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.1.0/hearth-1.1.0-amd64.dmg",
    );
    expect(first.windowsAmd64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.1.0/hearth-setup-1.1.0-amd64.exe",
    );
    expect(first.linuxDebArm64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.1.0/hearth_1.1.0_arm64.deb",
    );
    expect(first.checksumsUrl).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.1.0/SHA256SUMS.txt",
    );
    expect(second).toEqual(first);
  });

  it("keeps universal macOS DMG URLs for legacy v1.0.x redirect tags", async () => {
    const fakeFetch = async () =>
      new Response(null, {
        status: 302,
        headers: { Location: "https://github.com/Blinks44/hearth/releases/tag/v1.0.5" },
      });

    const resolved = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000);
    expect(resolved.macosArm64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.0.5/hearth-1.0.5.dmg",
    );
    expect(resolved.macosAmd64).toBe(
      "https://github.com/Blinks44/hearth/releases/download/v1.0.5/hearth-1.0.5.dmg",
    );
  });

  it("falls back cleanly to /releases/latest on HTTP 404 or 403", async () => {
    const fakeFetch = async () => new Response("Not Found", { status: 404 });
    const res = await fetchLatestRelease(fakeFetch as typeof fetch, 1000);

    expect(res).toEqual(fallbackReleaseAssets());
    expect(res.macosArm64).toBe(site.latestRelease);
    expect(res.macosAmd64).toBe(site.latestRelease);
    expect(res.isFallback).toBe(true);
  });

  it("falls back to GitHub API when redirect header is missing and caches result", async () => {
    let calls = 0;
    const fakeFetch = async (input: string | URL | Request) => {
      calls++;
      if (String(input) === site.latestRelease) {
        return new Response(null, { status: 200 });
      }
      return new Response(
        JSON.stringify({
          tag_name: "v1.0.5",
          html_url: "https://github.com/Blinks44/hearth/releases/tag/v1.0.5",
          assets: [{ name: "hearth-1.0.5.dmg", browser_download_url: "https://dl/hearth.dmg" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    };

    const first = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000);
    const second = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000 + 60_000);
    expect(calls).toBe(2);
    expect(first.macosArm64).toBe("https://dl/hearth.dmg");
    expect(first.macosAmd64).toBe("https://dl/hearth.dmg");
    expect(second.macosArm64).toBe("https://dl/hearth.dmg");
  });
});
