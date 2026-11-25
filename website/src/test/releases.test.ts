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
      tag_name: "v1.0.0",
      html_url: "https://github.com/Blinks44/hearth/releases/tag/v1.0.0",
      assets: [
        { name: "hearth-1.0.0.dmg", browser_download_url: "https://dl/hearth.dmg" },
        { name: "hearth-setup-1.0.0-amd64.exe", browser_download_url: "https://dl/win-x64.exe" },
        { name: "hearth-setup-1.0.0-arm64.exe", browser_download_url: "https://dl/win-arm64.exe" },
        { name: "hearth_1.0.0_amd64.deb", browser_download_url: "https://dl/linux-x64.deb" },
        { name: "hearth_1.0.0_arm64.deb", browser_download_url: "https://dl/linux-arm64.deb" },
        {
          name: "hearth-1.0.0-windows-amd64.tar.gz",
          browser_download_url: "https://dl/win-x64.tar.gz",
        },
        {
          name: "hearth-1.0.0-windows-arm64.tar.gz",
          browser_download_url: "https://dl/win-arm64.tar.gz",
        },
        {
          name: "hearth-1.0.0-linux-amd64.tar.gz",
          browser_download_url: "https://dl/linux-x64.tar.gz",
        },
        {
          name: "hearth-1.0.0-linux-arm64.tar.gz",
          browser_download_url: "https://dl/linux-arm64.tar.gz",
        },
        { name: "SHA256SUMS.txt", browser_download_url: "https://dl/SHA256SUMS.txt" },
      ],
    });

    expect(parsed.version).toBe("1.0.0");
    expect(parsed.isFallback).toBe(false);
    expect(parsed.macosDmg).toBe("https://dl/hearth.dmg");
    expect(parsed.windowsAmd64).toBe("https://dl/win-x64.exe");
    expect(parsed.windowsArm64).toBe("https://dl/win-arm64.exe");
    expect(parsed.linuxDebAmd64).toBe("https://dl/linux-x64.deb");
    expect(parsed.linuxDebArm64).toBe("https://dl/linux-arm64.deb");
    expect(parsed.linuxTarAmd64).toBe("https://dl/linux-x64.tar.gz");
    expect(parsed.linuxTarArm64).toBe("https://dl/linux-arm64.tar.gz");
    expect(parsed.checksumsUrl).toBe("https://dl/SHA256SUMS.txt");
  });

  it("falls back cleanly to /releases/latest on HTTP 404 or 403", async () => {
    const fakeFetch = async () => new Response("Not Found", { status: 404 });
    const res = await fetchLatestRelease(fakeFetch as typeof fetch, 1000);

    expect(res).toEqual(fallbackReleaseAssets());
    expect(res.macosDmg).toBe(site.latestRelease);
    expect(res.isFallback).toBe(true);
  });

  it("caches successful responses within TTL without re-fetching", async () => {
    let calls = 0;
    const fakeFetch = async () => {
      calls++;
      return new Response(
        JSON.stringify({
          tag_name: "v1.0.0",
          html_url: "https://github.com/Blinks44/hearth/releases/tag/v1.0.0",
          assets: [{ name: "hearth-1.0.0.dmg", browser_download_url: "https://dl/hearth.dmg" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    };

    const first = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000);
    const second = await fetchLatestRelease(fakeFetch as typeof fetch, 10_000 + 60_000);
    expect(calls).toBe(1);
    expect(first.macosDmg).toBe("https://dl/hearth.dmg");
    expect(second.macosDmg).toBe("https://dl/hearth.dmg");
  });
});
