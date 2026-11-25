import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ArticleBody } from "./article-body";

describe("ArticleBody", () => {
  it("renders HTML as markup", () => {
    render(<ArticleBody html="<p>Hello <strong>world</strong></p>" text="Hello world" fallback="none" />);

    expect(screen.getByText("world").tagName).toBe("STRONG");
  });

  it("falls back to plain text", () => {
    render(<ArticleBody html={null} text="Just <b>text</b>" fallback="none" />);

    expect(screen.getByText("Just <b>text</b>")).toBeInTheDocument();
  });

  it("shows the fallback when there is no content", () => {
    render(<ArticleBody html="" text={null} fallback={<p>Nothing here</p>} />);

    expect(screen.getByText("Nothing here")).toBeInTheDocument();
  });
});
