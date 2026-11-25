import { Check, Copy } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import * as m from "@/integrations/i18n/paraglide/messages";

interface CopyButtonProps {
  text: string;
  label?: string;
  className?: string;
  showLabelWhenIdle?: boolean;
}

export function CopyButton({
  text,
  label,
  className = "copy-pill-btn",
  showLabelWhenIdle = false,
}: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const onCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
    }
    setCopied(true);
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), 1800);
  }, [text]);

  return (
    <button
      type="button"
      className={className}
      data-copied={copied}
      onClick={onCopy}
      aria-label={copied ? m.heroCopied() : label || m.heroCopyCommand()}
    >
      {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
      {copied ? (
        <span>{m.heroCopied()}</span>
      ) : showLabelWhenIdle && label ? (
        <span>{label}</span>
      ) : null}
    </button>
  );
}
