import { describe, expect, it } from "vitest";
import { site } from "@/config/site";
import { getInstallCommand } from "@/lib/os";

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
