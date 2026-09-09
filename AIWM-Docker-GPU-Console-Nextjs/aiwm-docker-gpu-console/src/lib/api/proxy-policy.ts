// Match the browser origin against the inbound Host; Next's internal URL may use localhost.
export function allowedMutationOrigin(origin: string | null, host: string | null): boolean {
  if (!origin) return true;
  try {
    const url = new URL(origin);
    return ["http:", "https:"].includes(url.protocol) && url.origin === origin && url.host === host;
  } catch {
    return false;
  }
}

// Only the Control Plane public contract may cross the browser BFF boundary.
export function allowedPublicRoute(method: string, path: string[]): boolean {
  if (!path.length || path.some((part) => !/^[A-Za-z0-9_-]+$/.test(part))) return false;
  const route = path.join("/");
  if (method === "GET") return /^(system\/summary|servers(?:\/[A-Za-z0-9_-]+)?|gpus|containers|jobs(?:\/[A-Za-z0-9_-]+)?|queue)$/.test(route);
  if (method === "POST") return /^(jobs|jobs\/preview|jobs\/[A-Za-z0-9_-]+\/stop|servers\/[A-Za-z0-9_-]+\/drain|scheduler\/run-once)$/.test(route);
  return false;
}
