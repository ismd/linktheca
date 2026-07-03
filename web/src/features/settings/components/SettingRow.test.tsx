import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { SettingRow } from "./SettingRow";

describe("SettingRow", () => {
  it("renders label and value", () => {
    render(<SettingRow label="Email" value="claude@ismd.dev" />);
    expect(screen.getByText("Email")).toBeInTheDocument();
    expect(screen.getByText("claude@ismd.dev")).toBeInTheDocument();
  });
});
