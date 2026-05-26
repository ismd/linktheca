import { relativeFromNow } from "@/features/library/time";

export function fmtSweep(d: Date | null): string {
  return d ? `Last sweep · ${relativeFromNow(d)}` : "Awaiting first sweep";
}

export function fmtLastMatch(d: Date | null): string {
  return d ? relativeFromNow(d) : "—";
}
