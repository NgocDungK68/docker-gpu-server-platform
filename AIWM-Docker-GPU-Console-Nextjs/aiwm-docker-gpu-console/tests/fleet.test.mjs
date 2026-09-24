import test from "node:test";
import assert from "node:assert/strict";
import { fleetMetrics } from "../src/features/dashboard/fleet.ts";

const gpu = (state, utilizationPct, healthy = true) => ({ state, utilizationPct, healthy });
const server = (gpus, extra = {}) => ({ status: "ONLINE", inventoryReceivedAt: new Date().toISOString(), schedulable: true, gpus, ...extra });

test("fleet averages valid GPU samples including zero, not per-server averages", () => {
  const s = fleetMetrics([
    server([gpu("FREE", 0), gpu("ALLOCATED", 80)]),
    server([gpu("OCCUPIED_LEGACY", 40)]),
    server([gpu("OCCUPIED_UNKNOWN", null), gpu("FREE", NaN), gpu("FREE", 101), gpu("UNHEALTHY", 10, false)]),
    server([gpu("ALLOCATED", 100)], { status: "OFFLINE", schedulable: false }),
    server([gpu("FREE", 90)], { inventoryReceivedAt: undefined }),
  ]);
  assert.equal(s.telemetryCount, 3);
  assert.equal(s.averageUtilization, 40);
  assert.equal(s.allocated, 2); // Last known allocation is still retained for offline hosts.
  assert.equal(s.legacy, 1);
  assert.equal(s.unknown, 1);
  assert.equal(s.unhealthy, 1);
});
test("drained readings remain visible but free GPUs are not ready; empty mean is unavailable", () => {
  const s = fleetMetrics([server([gpu("FREE", 60), gpu("RESERVED", 0)], { status: "DRAINING", schedulable: false })]);
  assert.equal(s.free, 0);
  assert.equal(s.reserved, 1);
  assert.equal(s.averageUtilization, 30);
  assert.equal(fleetMetrics([]).averageUtilization, null);
});
