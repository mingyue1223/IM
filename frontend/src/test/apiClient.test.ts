import { describe, expect, it, vi } from "vitest";
// @vitest-environment node

import { GoIMApiClient } from "../../api/client";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

describe("GoIMApiClient", () => {
  it("adds the access token and unwraps successful data", async () => {
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer access-token");
      return jsonResponse({ code: 0, message: "ok", data: { id: 7 } });
    });
    const client = new GoIMApiClient({ baseUrl: "http://example.test/api", getAccessToken: () => "access-token", fetch: fetchMock as typeof fetch });
    await expect(client.get<{ id: number }>("/item")).resolves.toEqual({ id: 7 });
  });

  it("refreshes once and retries an unauthorized request", async () => {
    let requestCount = 0;
    const recover = vi.fn(async () => true);
    const fetchMock = vi.fn(async () => {
      requestCount += 1;
      return requestCount === 1 ? jsonResponse({ code: 1003, message: "expired" }, 401) : jsonResponse({ code: 0, message: "ok", data: "done" });
    });
    const client = new GoIMApiClient({ baseUrl: "http://example.test", onUnauthorized: recover, fetch: fetchMock as typeof fetch });
    await expect(client.get<string>("/protected")).resolves.toBe("done");
    expect(recover).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("throws the API error without an endless retry", async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ code: 1106, message: "invalid token" }, 401));
    const client = new GoIMApiClient({ baseUrl: "http://example.test", onUnauthorized: async () => true, fetch: fetchMock as typeof fetch });
    await expect(client.get("/protected")).rejects.toMatchObject({ code: 1106, status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
