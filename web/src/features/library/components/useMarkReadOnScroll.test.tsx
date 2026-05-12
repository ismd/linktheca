import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMarkReadOnScroll } from "./useMarkReadOnScroll";

function setScrollMetrics(scrollTop: number, scrollHeight: number, clientHeight: number) {
  Object.defineProperty(document.documentElement, "scrollTop", {
    configurable: true,
    value: scrollTop,
  });
  Object.defineProperty(document.documentElement, "scrollHeight", {
    configurable: true,
    value: scrollHeight,
  });
  Object.defineProperty(document.documentElement, "clientHeight", {
    configurable: true,
    value: clientHeight,
  });
}

describe("useMarkReadOnScroll", () => {
  beforeEach(() => {
    setScrollMetrics(0, 1000, 500);
  });

  it("does not fire below threshold", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: true, onReach: fn }));
    setScrollMetrics(100, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).not.toHaveBeenCalled();
  });

  it("fires once at ≥90% of scrollable area", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: true, onReach: fn }));
    // scrollable area = scrollHeight - clientHeight = 500. 90% = 450.
    setScrollMetrics(460, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).toHaveBeenCalledTimes(1);

    setScrollMetrics(500, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).toHaveBeenCalledTimes(1); // still once
  });

  it("does nothing when disabled", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: false, onReach: fn }));
    setScrollMetrics(500, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).not.toHaveBeenCalled();
  });
});
