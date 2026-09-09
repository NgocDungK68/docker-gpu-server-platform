"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { GPUInventoryItem } from "@/lib/api/types";

const states = [
  { key: "FREE", label: "Sẵn sàng", color: "#10b981" },
  { key: "ALLOCATED", label: "AIWM đang dùng", color: "#0284c7" },
  { key: "OCCUPIED_LEGACY", label: "Workload cũ", color: "#7c3aed" },
  { key: "OCCUPIED_UNKNOWN", label: "Không xác định", color: "var(--brand-red)" },
  { key: "UNHEALTHY", label: "Không khoẻ", color: "#64748b" },
] as const;

export function GPUStateChart({ items }: { items: GPUInventoryItem[] }) {
  const data = states.map((state) => ({ ...state, value: items.filter((item) => item.gpu.state === state.key).length })).filter((item) => item.value > 0);
  return <div className="grid min-h-56 grid-cols-[minmax(150px,1fr)_minmax(150px,.8fr)] items-center gap-2 px-4 py-3"><div className="h-52"><ResponsiveContainer width="100%" height="100%"><PieChart><Pie data={data} dataKey="value" nameKey="label" innerRadius={55} outerRadius={78} paddingAngle={3} stroke="none">{data.map((item) => <Cell key={item.key} fill={item.color} />)}</Pie><Tooltip formatter={(value) => [`${value} GPU`, "Số lượng"]} contentStyle={{ borderRadius: 10, borderColor: "#e2e8f0", fontSize: 13 }} /></PieChart></ResponsiveContainer></div><div className="space-y-3">{data.map((item) => <div key={item.key} className="flex items-center gap-2.5 text-sm"><span className="size-2.5 rounded-sm" style={{ background: item.color }} /><span className="min-w-0 flex-1 text-slate-600">{item.label}</span><b className="text-slate-950">{item.value}</b></div>)}</div></div>;
}
