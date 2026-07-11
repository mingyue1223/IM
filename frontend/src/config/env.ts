const defaultApiBaseUrl = "http://localhost:18080/api/v1";
const defaultWsUrl = "ws://localhost:18080/ws";
const defaultStaticBaseUrl = "http://localhost:18080";

function trimTrailingSlash(value: string) {
  return value.replace(/\/$/, "");
}

export const env = {
  apiBaseUrl: trimTrailingSlash(import.meta.env.VITE_API_BASE_URL || defaultApiBaseUrl),
  wsUrl: trimTrailingSlash(import.meta.env.VITE_WS_URL || defaultWsUrl),
  staticBaseUrl: trimTrailingSlash(import.meta.env.VITE_STATIC_BASE_URL || defaultStaticBaseUrl),
} as const;
