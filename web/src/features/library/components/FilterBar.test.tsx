import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FilterBar } from "./FilterBar";

describe("FilterBar", () => {
  it("highlights the active state pill", () => {
    render(
      <FilterBar
        state={undefined}
        favorite={false}
        onChange={() => {}}
      />,
    );
    const unread = screen.getByRole("button", { name: /^unread$/i });
    expect(unread).toHaveAttribute("aria-pressed", "true");
    const all = screen.getByRole("button", { name: /^all$/i });
    expect(all).toHaveAttribute("aria-pressed", "false");
  });

  it("clicking a state pill calls onChange with its filter value", async () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        state={undefined}
        favorite={false}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /^read$/i }));
    expect(onChange).toHaveBeenLastCalledWith({ state: "read", favorite: undefined });

    await userEvent.click(screen.getByRole("button", { name: /^all$/i }));
    expect(onChange).toHaveBeenLastCalledWith({ state: "all", favorite: undefined });
  });

  it("toggling favorites flips the favorite flag", async () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        state={undefined}
        favorite={false}
        onChange={onChange}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /favorites only/i }),
    );
    expect(onChange).toHaveBeenLastCalledWith({ state: undefined, favorite: true });
  });
});
