import { NextResponse } from "next/server";
import { getServerConfig } from "@/config/server";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const config = getServerConfig();
    const response = await fetch(new URL("/healthz", config.backend), {
      cache: "no-store",
      signal: AbortSignal.timeout(config.requestTimeoutMs),
    });
    return NextResponse.json({ status: response.ok ? "ok" : "unreachable" });
  } catch {
    return NextResponse.json({ status: "unreachable" });
  }
}
