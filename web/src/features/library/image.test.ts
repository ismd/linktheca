import { describe, it, expect } from "vitest";
import { previewFit } from "./image";

describe("previewFit", () => {
  it.each([
    ["og:image 1200×630", 1200, 630],
    ["16:9 photo", 1920, 1080],
    ["2:1 twitter card", 1200, 600],
    ["3:2 photo", 1500, 1000],
  ])("covers a %s", (_, w, h) => {
    expect(previewFit(w, h)).toBe("cover");
  });

  it.each([
    ["4:3 screenshot", 1024, 768],
    ["square logo", 512, 512],
    ["portrait shot", 800, 1200],
    ["ultra-wide strip", 1800, 500],
  ])("letterboxes a %s", (_, w, h) => {
    expect(previewFit(w, h)).toBe("contain");
  });

  it("falls back to cover when the size is unknown", () => {
    expect(previewFit(0, 0)).toBe("cover");
  });
});
