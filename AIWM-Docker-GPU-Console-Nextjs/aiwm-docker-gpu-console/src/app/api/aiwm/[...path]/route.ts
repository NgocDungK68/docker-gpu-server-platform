import { NextRequest, NextResponse } from "next/server";
import { getServerConfig } from "@/config/server";
import { allowedMutationOrigin, allowedPublicRoute } from "@/lib/api/proxy-policy";

export const dynamic = "force-dynamic";
type RouteContext = { params: Promise<{ path: string[] }> };

async function proxy(request: NextRequest, context: RouteContext) {
  const { path } = await context.params;
  if (!allowedPublicRoute(request.method, path)) return NextResponse.json({ error: { code: "NOT_FOUND", message: "Public API route not found" } }, { status: 404 });
  if (request.method !== "GET" && !allowedMutationOrigin(request.headers.get("origin"), request.headers.get("host"))) return NextResponse.json({ error: { code: "FORBIDDEN", message: "Cross-origin mutation rejected" } }, { status: 403 });
  try {
    const config = getServerConfig();
    const login = path.join("/") === "auth/login";
    const logout = path.join("/") === "auth/logout";
    const token = request.cookies.get("aiwm_session")?.value;
    if (!login && !token) return NextResponse.json({ error: { code: "UNAUTHORIZED", message: "Vui lòng đăng nhập." } }, { status: 401 });
    const upstream = new URL("/api/v1/" + path.join("/"), config.backend);
    upstream.search = request.nextUrl.search;
    const body = request.method === "GET" ? undefined : await request.arrayBuffer();
    if (body && body.byteLength > 1048576) return NextResponse.json({ error: { code: "BODY_TOO_LARGE", message: "Request exceeds 1 MiB" } }, { status: 413 });
    const response = await fetch(upstream, {
      method: request.method,
      headers: { Accept: "application/json", "Content-Type": "application/json", ...(token ? { Authorization: "Bearer " + token } : {}) },
      body, cache: "no-store", redirect: "error", signal: AbortSignal.timeout(config.requestTimeoutMs),
    });
    if (login && response.ok) {
      const payload = await response.json();
      const { token: sessionToken, ...data } = payload.data;
      if (typeof sessionToken !== "string") throw new Error("Invalid session response");
      const result = NextResponse.json({ data }, { headers: { "Cache-Control": "no-store" } });
      result.cookies.set("aiwm_session", sessionToken, { httpOnly: true, sameSite: "strict", secure: config.secureCookies, path: "/", expires: new Date(data.expiresAt) });
      return result;
    }
    const result = new NextResponse(response.body, { status: response.status, headers: { "Content-Type": "application/json", "Cache-Control": "no-store" } });
    if ((logout && response.ok) || response.status === 401) result.cookies.delete("aiwm_session");
    return result;
  } catch {
    return NextResponse.json({ error: { code: "CONTROL_PLANE_UNREACHABLE", message: "Không kết nối được AIWM Control Plane." } }, { status: 502 });
  }
}
export const GET = proxy;
export const POST = proxy;
