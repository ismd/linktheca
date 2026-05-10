import { describe, it, expect } from "vitest";

describe("test setup", () => {
  it("can run a basic assertion", () => {
    expect(1 + 1).toBe(2);
  });

  it("has DOM available", () => {
    const el = document.createElement("div");
    el.textContent = "hello";
    expect(el.textContent).toBe("hello");
  });
});
