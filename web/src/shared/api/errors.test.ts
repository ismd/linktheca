import { describe, it, expect } from "vitest";
import { ApiError } from "./errors";

describe("ApiError", () => {
  it("captures status, code, message, and details", () => {
    const err = new ApiError(422, "validation_failed", "Invalid input", { field: "email" });
    expect(err.status).toBe(422);
    expect(err.code).toBe("validation_failed");
    expect(err.message).toBe("Invalid input");
    expect(err.details).toEqual({ field: "email" });
  });

  it("is an instance of Error", () => {
    const err = new ApiError(500, "internal", "boom");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ApiError");
  });
});
