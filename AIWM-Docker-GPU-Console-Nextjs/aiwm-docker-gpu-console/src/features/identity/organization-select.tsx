"use client";

import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, Search } from "lucide-react";
import type { Organization } from "@/lib/api/identity";

export function OrganizationSelect({ organizations, value, onChange, allowAll = false, compact = false }: { organizations: Organization[]; value: string; onChange: (id: string) => void; allowAll?: boolean; compact?: boolean }) {
  const [search,setSearch] = useState("");
  const dropdown = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    if (!compact) return;
    const closeOutside = (event: PointerEvent) => {
      if (dropdown.current && event.target instanceof Node && !dropdown.current.contains(event.target)) dropdown.current.open = false;
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [compact]);
  const filtered = organizations.filter(o => o.id === value || `${o.code} ${o.name}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
  const selected = organizations.find(o => o.id === value);
  if (compact) {
    const choose = (id: string) => {
      onChange(id);
      if (dropdown.current) {
        dropdown.current.open = false;
        dropdown.current.querySelector("summary")?.focus();
      }
      setSearch("");
    };
    return <details ref={dropdown} className="group relative" onKeyDown={event => {
      if (event.key === "Escape" && dropdown.current) {
        dropdown.current.open = false;
        dropdown.current.querySelector("summary")?.focus();
      }
    }} onToggle={event => { if (!event.currentTarget.open) setSearch(""); }}>
      <summary aria-label="Lọc theo đơn vị" className="btn btn-secondary organization-filter-trigger cursor-pointer gap-3"><span className="truncate">{selected ? selected.code : value ? "Đơn vị đã chọn" : "Tất cả đơn vị"}</span><ChevronDown className="size-4 shrink-0 text-slate-400 transition-transform group-open:rotate-180" /></summary>
      <div className="absolute right-0 z-40 mt-2 w-72 max-w-[calc(100vw-2rem)] rounded-xl border border-slate-200 bg-white p-2 shadow-lg">
        <div className="relative mb-2"><Search className="pointer-events-none absolute left-3 top-3 size-4 text-slate-400" /><input type="search" className="field-input organization-filter-search" aria-label="Tìm đơn vị theo mã hoặc tên" placeholder="Tìm đơn vị…" value={search} onChange={e => setSearch(e.target.value)} /></div>
        <div className="max-h-64 space-y-1 overflow-y-auto" aria-label="Danh sách đơn vị">
          {allowAll && <button type="button" className="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm font-semibold hover:bg-slate-50" aria-pressed={!value} onClick={() => choose("")}>Tất cả đơn vị{!value && <Check className="size-4 text-red-600" />}</button>}
          {filtered.map(o => <button key={o.id} type="button" className="flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left hover:bg-slate-50" aria-pressed={o.id === value} onClick={() => choose(o.id)}><span className="min-w-0"><span className="block text-sm font-semibold text-slate-900">{o.code}{!o.enabled && " (Tạm khóa)"}</span><span className="block truncate text-xs text-slate-500">{o.name}</span></span>{o.id === value && <Check className="size-4 shrink-0 text-red-600" />}</button>)}
          {!filtered.length && <p className="p-3 text-xs text-slate-500">Không tìm thấy đơn vị phù hợp.</p>}
        </div>
      </div>
    </details>;
  }
  return <div className="space-y-2 rounded-xl border border-slate-200 bg-slate-50 p-3">
    <input type="search" className="field-input" aria-label="Tìm đơn vị theo mã hoặc tên" placeholder="Tìm mã hoặc tên đơn vị…" value={search} onChange={e => setSearch(e.target.value)} />
    <select className="field-select" aria-label="Đơn vị" required={!allowAll} value={value} onChange={e => onChange(e.target.value)}>
      <option value="">{allowAll ? "Tất cả đơn vị" : "Chọn đơn vị"}</option>
      {filtered.map(o => <option key={o.id} value={o.id}>{o.code} · {o.name}{o.enabled ? "" : " (Tạm khóa)"}</option>)}
    </select>
    {!filtered.length && <p className="text-xs text-slate-500">Không tìm thấy đơn vị phù hợp.</p>}
  </div>;
}
