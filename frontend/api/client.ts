import type { ApiResponse } from "../goim-api-types";

export interface ApiClientOptions {
  baseUrl: string;
  getAccessToken?: () => string | null | Promise<string | null>;
  onUnauthorized?: () => void | Promise<void>;
  fetch?: typeof fetch;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: number,
    readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class GoIMApiClient {
  private readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;

  constructor(private readonly options: ApiClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.fetchImpl = options.fetch ?? fetch;
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const token = await this.options.getAccessToken?.();
    const headers = new Headers(init.headers);
    if (token) headers.set("Authorization", `Bearer ${token}`);

    const response = await this.fetchImpl(`${this.baseUrl}${path}`, { ...init, headers });
    const isJson = response.headers.get("content-type")?.includes("application/json");
    const body: ApiResponse<T> | undefined = isJson ? await response.json() : undefined;

    if (response.status === 401) await this.options.onUnauthorized?.();
    if (!response.ok || !body || body.code !== 0) {
      throw new ApiError(body?.message ?? response.statusText, body?.code ?? -1, response.status);
    }
    return body.data as T;
  }

  get<T>(path: string): Promise<T> { return this.request<T>(path); }
  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: "POST",
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }
  put<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>(path, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  }
  delete<T>(path: string): Promise<T> { return this.request<T>(path, { method: "DELETE" }); }
  upload<T>(path: string, form: FormData): Promise<T> { return this.request<T>(path, { method: "POST", body: form }); }
}
