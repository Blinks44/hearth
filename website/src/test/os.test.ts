import { afterEach, describe, expect, it, vi } from "vitest";
import { site } from "@/config/site";
import { detectPlatform, getInstallCommand } from "@/lib/os";

describe("detectPlatform", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("detects Intel macOS when userAgentData reports x86", () => {
    vi.stubGlobal("navigator", {
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
      platform: "MacIntel",
      userAgentData: { platform: "macOS", architecture: "x86" },
    });
    expect(detectPlatform()).toEqual({ os: "macos", arch: "amd64" });
  });

  it("defaults macOS to arm64 without an explicit x86 architecture hint", () => {
    vi.stubGlobal("navigator", {
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
      platform: "MacIntel",
    });
    expect(detectPlatform()).toEqual({ os: "macos", arch: "arm64" });
  });
});

describe("getInstallCommand", () => {
  it("returns brew cask command with $ prompt for macOS", () => {
    expect(getInstallCommand("macos")).toEqual({
      prompt: "$",
      command: site.brewCommand,
    });
  });

  it("returns install.sh one-liner with $ prompt for Linux", () => {
    expect(getInstallCommand("linux")).toEqual({
      prompt: "$",
      command: site.linuxInstallCommand,
    });
  });

  it("returns install.ps1 one-liner with > prompt for Windows", () => {
    expect(getInstallCommand("windows")).toEqual({
      prompt: ">",
      command: site.windowsInstallCommand,
    });
  });
});
