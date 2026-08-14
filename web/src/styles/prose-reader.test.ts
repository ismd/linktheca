/// <reference types="node" />
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { cwd } from "node:process";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(cwd(), "src/styles/globals.css"), "utf8");

/** Returns the declaration block of the first rule with the given selector. */
function ruleBody(selector: string): string {
  const start = css.indexOf(`${selector} {`);
  if (start === -1) return "";
  const open = css.indexOf("{", start);
  const close = css.indexOf("}", open);
  return css.slice(open + 1, close);
}

describe("prose-reader code blocks", () => {
  // jsdom has no layout engine, so overflow can only be guarded at the CSS
  // source level: <pre> defaults to white-space: pre and will widen the reader
  // column (and the page) past the viewport unless it is explicitly clamped.
  it("clamps <pre> to the reader column and scrolls it internally", () => {
    const body = ruleBody(".prose-reader pre");

    expect(body).not.toBe("");
    expect(body).toMatch(/max-width:\s*100%/);
    expect(body).toMatch(/overflow-x:\s*auto/);
  });

  it("does not paint the inline-code pill on <code> inside <pre>", () => {
    const body = ruleBody(".prose-reader pre code");

    expect(body).not.toBe("");
    expect(body).toMatch(/background:\s*none/);
    expect(body).toMatch(/border:\s*0/);
  });
});