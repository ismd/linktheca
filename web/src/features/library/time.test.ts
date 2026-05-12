import { describe, it, expect, vi, afterEach } from "vitest";
import { relativeFromNow, readingTimeLabel } from "./time";

const fixedNow = new Date("2026-05-11T12:00:00Z");

afterEach(() => {
  vi.useRealTimers();
});

describe("relativeFromNow", () => {
  it("returns 'today' for today", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-11T08:00:00Z");
    expect(relativeFromNow(d)).toBe("today");
  });

  it("returns 'yesterday' for ~24h ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-10T12:00:00Z");
    expect(relativeFromNow(d)).toBe("yesterday");
  });

  it("returns '3 days ago' for 3 days ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-08T12:00:00Z");
    expect(relativeFromNow(d)).toBe("3 days ago");
  });

  it("returns 'Apr 11' for >7 days ago in same year", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-04-11T12:00:00Z");
    expect(relativeFromNow(d)).toBe("Apr 11");
  });

  it("returns 'Apr 11, 2025' for previous year", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2025-04-11T12:00:00Z");
    expect(relativeFromNow(d)).toBe("Apr 11, 2025");
  });
});

describe("readingTimeLabel", () => {
  it("returns '1 min read' for <90s", () => {
    expect(readingTimeLabel(45)).toBe("1 min read");
    expect(readingTimeLabel(89)).toBe("1 min read");
  });

  it("rounds to nearest minute", () => {
    expect(readingTimeLabel(180)).toBe("3 min read");
    expect(readingTimeLabel(330)).toBe("6 min read");
  });

  it("returns '— read' for null", () => {
    expect(readingTimeLabel(null)).toBe("— read");
  });
});
