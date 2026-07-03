import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { useAuthStore } from "@/features/auth/store";
import { AccountSection } from "./AccountSection";

beforeEach(() => {
  useAuthStore.getState().clearSession();
});

describe("AccountSection", () => {
  it("renders name, email, and Member role for a non-admin user", () => {
    useAuthStore.getState().setSession("tok", {
      id: 1, email: "claude@ismd.dev", displayName: "ismd", isAdmin: false,
    });
    render(<AccountSection />);
    expect(screen.getByText("ismd")).toBeInTheDocument();
    expect(screen.getByText("claude@ismd.dev")).toBeInTheDocument();
    expect(screen.getByText("Member")).toBeInTheDocument();
  });

  it("renders Administrator role for an admin user", () => {
    useAuthStore.getState().setSession("tok", {
      id: 2, email: "admin@ismd.dev", displayName: "Admin", isAdmin: true,
    });
    render(<AccountSection />);
    expect(screen.getByText("Administrator")).toBeInTheDocument();
  });

  it("renders nothing when there is no user", () => {
    const { container } = render(<AccountSection />);
    expect(container).toBeEmptyDOMElement();
  });
});
