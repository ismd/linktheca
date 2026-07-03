import { describe, it, expect } from "vitest";
import { shouldRetryQuery } from "./query-client";
import { ApiError } from "./errors";

describe("shouldRetryQuery", () => {
  it("does not retry 501 Not Implemented (retrying an unimplemented endpoint is pointless)", () => {
    expect(shouldRetryQuery(0, new ApiError(501, "not_implemented", "no"))).toBe(false);
  });

  it("retries transient 5xx up to 2 times", () => {
    expect(shouldRetryQuery(0, new ApiError(503, "unavailable", "x"))).toBe(true);
    expect(shouldRetryQuery(1, new ApiError(500, "internal", "x"))).toBe(true);
    expect(shouldRetryQuery(2, new ApiError(500, "internal", "x"))).toBe(false);
  });

  it("never retries 4xx (incl. 403 radar_disabled) or non-ApiError failures", () => {
    expect(shouldRetryQuery(0, new ApiError(403, "radar_disabled", "off"))).toBe(false);
    expect(shouldRetryQuery(0, new ApiError(404, "not_found", "x"))).toBe(false);
    expect(shouldRetryQuery(0, new Error("network"))).toBe(false);
  });
});