import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { apiFetch, ApiError, clearToken, getToken, setToken } from "./apiClient";

function mockFetchOnce(response: Partial<Response> & { json?: () => Promise<unknown> }) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({}),
    ...response,
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("apiClient token storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns null when no token is stored", () => {
    expect(getToken()).toBeNull();
  });

  it("round-trips a token through setToken/getToken", () => {
    setToken("abc123");
    expect(getToken()).toBe("abc123");
  });

  it("clearToken removes the stored token", () => {
    setToken("abc123");
    clearToken();
    expect(getToken()).toBeNull();
  });
});

describe("apiFetch", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  it("attaches the Authorization header when a token is present", async () => {
    setToken("my-token");
    const fetchMock = mockFetchOnce({ json: async () => ({ ok: true }) });

    await apiFetch("/notifications");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer my-token");
  });

  it("omits the Authorization header when no token is present", async () => {
    const fetchMock = mockFetchOnce({ json: async () => ({ ok: true }) });

    await apiFetch("/notifications");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("Authorization")).toBeNull();
  });

  it("returns undefined for a 204 No Content response", async () => {
    mockFetchOnce({ status: 204 });

    const result = await apiFetch("/tasks/1/pin", { method: "POST" });
    expect(result).toBeUndefined();
  });

  it("returns the parsed JSON body on success", async () => {
    mockFetchOnce({ json: async () => ({ id: "abc", name: "test" }) });

    const result = await apiFetch<{ id: string; name: string }>("/workspaces/1");
    expect(result).toEqual({ id: "abc", name: "test" });
  });

  it("throws ApiError with the status and parsed error body on failure", async () => {
    mockFetchOnce({ ok: false, status: 400, json: async () => ({ error: "bad_request" }) });

    await expect(apiFetch("/tasks/1")).rejects.toMatchObject({
      status: 400,
      message: "bad_request",
    });
  });

  it("ApiError is thrown as an instance of ApiError, not a generic Error", async () => {
    mockFetchOnce({ ok: false, status: 404, json: async () => ({ error: "not_found" }) });

    try {
      await apiFetch("/tasks/missing");
      expect.unreachable("apiFetch should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
    }
  });

  it("falls back to a generic message when the error response has no JSON body", async () => {
    mockFetchOnce({
      ok: false,
      status: 500,
      json: async () => {
        throw new Error("not json");
      },
    });

    await expect(apiFetch("/tasks/1")).rejects.toMatchObject({
      status: 500,
      message: "request failed with status 500",
    });
  });
});
