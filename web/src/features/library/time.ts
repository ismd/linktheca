import {
  differenceInCalendarDays,
  format,
  isSameYear,
} from "date-fns";

export function relativeFromNow(d: Date, now: Date = new Date()): string {
  const days = differenceInCalendarDays(now, d);
  if (days <= 0) return "today";
  if (days === 1) return "yesterday";
  if (days < 7) return `${days} days ago`;
  return isSameYear(d, now) ? format(d, "MMM d") : format(d, "MMM d, yyyy");
}

export function readingTimeLabel(seconds: number | null): string {
  if (seconds == null) return "— read";
  const minutes = Math.max(1, Math.round(seconds / 60));
  return `${minutes} min read`;
}
