"use client";

import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { Server } from "@/lib/api/types";

export function ServerCapacityChart({ servers }: { servers: Server[] }) {
  const data = servers.map((server) => ({
    name: server.name.replace("gpu-", ""),
    free: server.schedulable ? server.gpus.filter((gpu) => gpu.healthy && gpu.state === "FREE").length : 0,
    unavailable: server.gpus.filter((gpu) => gpu.state === "UNHEALTHY" || (!server.schedulable && gpu.state === "FREE")).length,
    managed: server.gpus.filter((gpu) => ["ALLOCATED", "RESERVED"].includes(gpu.state)).length,
    protected: server.gpus.filter((gpu) => ["OCCUPIED_LEGACY", "OCCUPIED_UNKNOWN"].includes(gpu.state)).length,
  }));
  return <div className="h-64 px-3 pb-3 pt-5"><ResponsiveContainer width="100%" height="100%"><BarChart data={data} margin={{ top: 4, right: 12, left: -22, bottom: 0 }}><CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#e7eaf0" /><XAxis dataKey="name" tick={{ fontSize: 11, fill: "#64748b" }} tickLine={false} axisLine={false} /><YAxis allowDecimals={false} tick={{ fontSize: 11, fill: "#64748b" }} tickLine={false} axisLine={false} /><Tooltip contentStyle={{ borderRadius: 10, borderColor: "#e2e8f0", fontSize: 13 }} /><Bar dataKey="managed" name="AIWM" stackId="gpu" fill="#0284c7" radius={[0, 0, 0, 0]} /><Bar dataKey="protected" name="Đang bảo vệ" stackId="gpu" fill="#7c3aed" /><Bar dataKey="unavailable" name="Chưa sẵn sàng" stackId="gpu" fill="#94a3b8" /><Bar dataKey="free" name="Sẵn sàng" stackId="gpu" fill="#10b981" radius={[5, 5, 0, 0]} /></BarChart></ResponsiveContainer></div>;
}
