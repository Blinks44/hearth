import { site } from "@/config/site";

export type PlatformOS = "macos" | "windows" | "linux";
export type PlatformArch = "arm64" | "amd64";

export interface DetectedPlatform {
  os: PlatformOS;
  arch: PlatformArch;
}

export interface PlatformInstallCommand {
  prompt: "$" | ">";
  command: string;
}

interface NavigatorUAData {
  platform?: string;
  architecture?: string;
}

export function detectPlatform(): DetectedPlatform {
  if (typeof navigator === "undefined") {
    return { os: "macos", arch: "arm64" };
  }

  const nav = navigator as Navigator & { userAgentData?: NavigatorUAData };
  const ua = (nav.userAgent || "").toLowerCase();
  const platform = (nav.userAgentData?.platform || nav.platform || "").toLowerCase();
  const archHint = (nav.userAgentData?.architecture || "").toLowerCase();

  let os: PlatformOS = "macos";
  if (platform.includes("win") || ua.includes("windows")) {
    os = "windows";
  } else if ((platform.includes("linux") || ua.includes("linux")) && !ua.includes("android")) {
    os = "linux";
  } else {
    os = "macos";
  }

  let arch: PlatformArch = "amd64";
  if (archHint.includes("x86") || ua.includes("x86_64") || ua.includes("x64")) {
    arch = "amd64";
  } else if (
    archHint.includes("arm") ||
    ua.includes("arm64") ||
    ua.includes("aarch64") ||
    os === "macos"
  ) {
    arch = "arm64";
  }

  return { os, arch };
}

export function getInstallCommand(os: PlatformOS): PlatformInstallCommand {
  if (os === "windows") {
    return {
      prompt: ">",
      command: site.windowsInstallCommand,
    };
  }
  if (os === "linux") {
    return {
      prompt: "$",
      command: site.linuxInstallCommand,
    };
  }
  return {
    prompt: "$",
    command: site.brewCommand,
  };
}
