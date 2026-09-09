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
    if (!config.apiToken) return NextResponse.json({ error: { code: "CONFIGURATION", message: "Chưa cấu hình AIWM_API_TOKEN trên Next.js server" } }, { status: 503 });
    const upstream = new URL("/api/v1/" + path.join("/"), config.backend);
    upstream.search = request.nextUrl.search;
    const body = request.method === "GET" ? undefined : await request.arrayBuffer();
    if (body && body.byteLength > 1048576) return NextResponse.json({ error: { code: "BODY_TOO_LARGE", message: "Request exceeds 1 MiB" } }, { status: 413 });
    const response = await fetch(upstream, {
      method: request.method,
      headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: "Bearer " + config.apiToken },
      body, cache: "no-store", redirect: "error", signal: AbortSignal.timeout(config.requestTimeoutMs),
    });
    return new NextResponse(response.body, { status: response.status, headers: { "Content-Type": "application/json", "Cache-Control": "no-store" } });
  } catch {
    return NextResponse.json({ error: { code: "CONTROL_PLANE_UNREACHABLE", message: "Không kết nối được AIWM Control Plane." } }, { status: 502 });
  }
}
export const GET = proxy;
export const POST = proxy;
