"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowLeft, Box, Cpu, Loader2, Network, Server, SlidersHorizontal } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm, useWatch } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { useToast } from "@/components/ui/toast";
import { useCreateJob } from "@/features/workloads/use-workloads";
import type { CreateJobInput } from "@/lib/api/types";

import { jobFormSchema as schema, parseKeyValueLines, type JobFormValues as FormValues } from "@/lib/jobs/form-schema";

export function JobForm() {
  const router = useRouter();
  const mutation = useCreateJob();
  const { pushToast } = useToast();
  const { register, handleSubmit, control, formState: { errors } } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: "gpu-demo", image: "alpine:3.21", commandLines: "sh\n-c\nsleep 300", environmentLines: "", gpuCount: 1, gpuModel: "", minVramMiB: 0, cpuMilli: 1000, memoryMiB: 256, priority: 50, strategy: "best-fit", selectorLines: "" },
  });
  const values = useWatch({ control });
  const submit = handleSubmit((form) => {
    const input: CreateJobInput = {
      name: form.name.trim(), image: form.image.trim(), backend: "DOCKER",
      command: form.commandLines.split("\n").map((item) => item.trim()).filter(Boolean),
      environment: parseKeyValueLines(form.environmentLines),
      resources: { gpuCount: form.gpuCount, gpuModel: form.gpuModel.trim() || undefined, minVramMiB: form.minVramMiB, cpuMilli: form.cpuMilli, memoryMiB: form.memoryMiB, allowSharedGpu: false },
      priority: form.priority, serverSelector: parseKeyValueLines(form.selectorLines), strategy: form.strategy,
    };
    mutation.mutate(input, { onSuccess: (job) => { pushToast({ tone: "success", title: "Đã đưa workload vào hàng đợi", description: `${job.name} · priority ${job.priority}` }); router.push(`/workloads/${job.id}`); }, onError: (error) => pushToast({ tone: "error", title: "Không tạo được workload", description: error.message }) });
  });

  return <form onSubmit={submit} className="grid gap-5 xl:grid-cols-[1fr_360px]">
    <div className="space-y-5"><Card><CardHeader title="Container" description="Image và entry command Agent sẽ gửi tới Docker Engine" /><div className="grid gap-4 p-5 md:grid-cols-2"><Field label="Tên workload" error={errors.name?.message}><input className="field-input" {...register("name")} /></Field><Field label="Docker image" error={errors.image?.message}><input className="field-input font-mono" {...register("image")} /></Field><Field label="Command — mỗi đối số một dòng" className="md:col-span-2"><textarea rows={5} className="field-textarea font-mono" {...register("commandLines")} /></Field><Field label="Environment — KEY=value mỗi dòng" error={errors.environmentLines?.message} className="md:col-span-2"><textarea rows={3} className="field-textarea font-mono" {...register("environmentLines")} /></Field></div></Card>
      <Card><CardHeader title="Tài nguyên" description="Filter bắt buộc trước khi scheduler chấm điểm server" /><div className="grid gap-4 p-5 md:grid-cols-2 lg:grid-cols-3"><Field label="Số GPU" error={errors.gpuCount?.message}><input type="number" min={1} className="field-input" {...register("gpuCount", { valueAsNumber: true })} /></Field><Field label="GPU model"><input className="field-input" placeholder="Để trống = bất kỳ" {...register("gpuModel")} /></Field><Field label="VRAM tối thiểu / GPU (MiB)" error={errors.minVramMiB?.message}><input type="number" className="field-input" {...register("minVramMiB", { setValueAs: (value) => value === "" ? undefined : Number(value) })} /></Field><Field label="CPU (millicores)"><input type="number" className="field-input" {...register("cpuMilli", { setValueAs: (value) => value === "" ? undefined : Number(value) })} /></Field><Field label="RAM (MiB)"><input type="number" className="field-input" {...register("memoryMiB", { setValueAs: (value) => value === "" ? undefined : Number(value) })} /></Field><div><span className="field-label">GPU sharing</span><div className="flex min-h-[43px] items-center rounded-lg border border-slate-200 bg-slate-50 px-3 text-sm text-slate-500">Tắt trong Docker MVP</div></div></div></Card>
      <Card><CardHeader title="Scheduling" description="Priority quyết định thứ tự queue; strategy quyết định chọn server" /><div className="grid gap-4 p-5 md:grid-cols-2"><Field label="Priority (0–1000)"><input type="number" min={0} max={1000} className="field-input" {...register("priority", { valueAsNumber: true })} /></Field><Field label="Placement strategy"><select className="field-select" {...register("strategy")}><option value="best-fit">Best Fit — giảm phần dư</option><option value="bin-pack">Bin Pack — gom workload</option><option value="fragmentation-aware">Fragmentation-aware — giữ server trống</option><option value="first-fit">First Fit — nhanh, dễ giải thích</option></select></Field><Field label="Server selector — key=value mỗi dòng" error={errors.selectorLines?.message} className="md:col-span-2"><textarea rows={3} className="field-textarea font-mono" {...register("selectorLines")} /></Field></div></Card>
      <div className="flex items-center justify-between"><Link href="/workloads" className="btn btn-ghost"><ArrowLeft className="size-4" />Huỷ</Link><Button type="submit" disabled={mutation.isPending}>{mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Box className="size-4" />}{mutation.isPending ? "Đang gửi…" : "Gửi vào hàng đợi"}</Button></div>
    </div>
    <div className="space-y-5 xl:sticky xl:top-24 xl:self-start"><Card><CardHeader title="Bản xem trước" description="Payload gửi tới POST /api/v1/jobs" /><div className="space-y-4 p-5"><Preview icon={<Cpu className="size-4" />} label="GPU" value={`${values.gpuCount || 0} × ${values.gpuModel || "bất kỳ"}`} /><Preview icon={<Server className="size-4" />} label="Giới hạn" value={`${values.minVramMiB || 0} MiB VRAM · ${values.cpuMilli || 0}m CPU`} /><Preview icon={<SlidersHorizontal className="size-4" />} label="Queue" value={`Priority ${values.priority || 0} · ${values.strategy}`} /><Preview icon={<Network className="size-4" />} label="Selector" value={values.selectorLines || "Không giới hạn server"} /></div></Card><div className="rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-900"><b>Sau khi gửi:</b> job ở trạng thái QUEUED. Control Plane lọc GPU trống, reserve nguyên tử rồi Agent mới pull/start container.</div></div>
  </form>;
}

function Field({ label, error, className, children }: { label: string; error?: string; className?: string; children: React.ReactNode }) { return <label className={className}><span className="field-label">{label}</span>{children}{error && <span className="field-error">{error}</span>}</label>; }
function Preview({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) { return <div className="flex gap-3"><span className="mt-0.5 text-slate-400">{icon}</span><div className="min-w-0"><p className="text-xs font-semibold text-slate-400">{label}</p><p className="mt-0.5 whitespace-pre-line break-words text-sm font-semibold text-slate-800">{value}</p></div></div>; }
