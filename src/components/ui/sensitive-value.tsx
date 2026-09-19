"use client";

import { useState } from "react";

// =============================================================================
// SensitiveValue — nilai sensitif (IMEI / ICCID / credential) di-blur
// -----------------------------------------------------------------------------
// Default: nilai di-mask (hanya 4 digit terakhir terlihat) + efek blur.
// Hover (desktop) / klik (mobile) untuk melihat nilai penuh.
// =============================================================================

interface SensitiveValueProps {
  value: string;
  className?: string;
  /** Jumlah digit terakhir yang tetap terlihat (default 4). */
  visibleTail?: number;
}

function maskValue(value: string, tail: number): string {
  const digits = (value.match(/\d/g) ?? []).length;
  const tailDigits = value.slice(-tail).match(/\d/g)?.length ?? 0;
  const masked = "•".repeat(Math.max(1, digits - tailDigits));
  return `${masked}${value.slice(-tail)}`;
}

export function SensitiveValue({ value, className, visibleTail = 4 }: SensitiveValueProps) {
  const [show, setShow] = useState(false);

  if (!value) {
    return <span className={className}>-</span>;
  }

  return (
    <button
      type="button"
      aria-label={show ? "Sembunyikan nilai" : "Lihat nilai"}
      onClick={() => setShow((v) => !v)}
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
      title={show ? value : "Klik atau hover untuk melihat"}
      className={`inline-flex items-center gap-1 rounded px-0.5 text-left tabular-nums transition-all duration-200 select-none ${
        show ? "" : "cursor-help blur-[3px] hover:blur-none focus:outline-none focus:blur-none"
      } ${className ?? ""}`}
    >
      {show ? value : maskValue(value, visibleTail)}
    </button>
  );
}