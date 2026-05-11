import { describe, it, expect, beforeEach } from "vitest";
import { readRefreshToken, writeRefreshToken, clearRefreshToken } from "./storage";

describe("refresh token storage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("returns null when no token is stored", () => {
    expect(readRefreshToken()).toBeNull();
  });

  it("write then read returns the same value", () => {
    writeRefreshToken("abc");
    expect(readRefreshToken()).toBe("abc");
  });

  it("clear removes the token", () => {
    writeRefreshToken("abc");
    clearRefreshToken();
    expect(readRefreshToken()).toBeNull();
  });

  it("returns null and swallows error when localStorage throws", () => {
    const original = Storage.prototype.getItem;
    Storage.prototype.getItem = () => {
      throw new Error("disabled");
    };
    try {
      expect(readRefreshToken()).toBeNull();
    } finally {
      Storage.prototype.getItem = original;
    }
  });
});
