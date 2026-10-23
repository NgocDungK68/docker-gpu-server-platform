// Server-only configuration: never import this module from a client component.
export function getServerConfig() {
  const backend = new URL(process.env.AIWM_API_BASE_URL ?? "http://localhost:8080");
  if (!["http:", "https:"].includes(backend.protocol) || backend.username || backend.password) throw new Error("Invalid backend URL");
  const requestTimeoutMs = Number(process.env.AIWM_API_TIMEOUT_MS ?? 30000);
  if (!Number.isFinite(requestTimeoutMs) || requestTimeoutMs <= 0) throw new Error("Invalid API timeout");
  return { backend, requestTimeoutMs, secureCookies: process.env.AIWM_SESSION_COOKIE_SECURE === "true" };
}
