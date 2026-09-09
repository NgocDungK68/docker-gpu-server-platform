"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowLeft, Box, Loader2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";
import { useForm, useWatch } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { useToast } from "@/components/ui/toast";
import { useCreateJob } from "@/features/workloads/use-workloads";
import { api, ApiError } from "@/lib/api/api-client";
import type { AllocationOption, CreateJobInput } from "@/lib/api/types";
import { allocationFormSchema, allocationInput, type JobFormValues } from "@/lib/jobs/form-schema";
import { AllocationPreview } from "./allocation-preview";

const apiFields: Record<string, keyof JobFormValues> = {
  name: "name", image: "image", command: "commandLines", environment: "environmentLines",
  "resources.gpuCount": "gpuCount", "resources.minVramMiB": "minVramMiB",
  "resources.performanceProfile": "performanceProfile", "resources.fp8Required": "fp8Required",
  "resources.cpuMilli": "cpuMilli", "resources.memoryMiB": "memoryMiB",
  workloadType: "workloadType", necessityLevel: "necessityLevel", necessityReason: "necessityReason",
  necessityExplanation: "necessityExplanation", systemImportance: "systemImportance",
  neededAt: "neededAt", ttlSeconds: "ttlHours",
};

export function JobForm() {
  const router = useRouter();
  const mutation = useCreateJob();
  const { pushToast } = useToast();
  const catalog = useQuery({ queryKey: ["allocation-options"], queryFn: api.getAllocationOptions });
  const schema = useMemo(() => allocationFormSchema(catalog.data), [catalog.data]);
  const { register, handleSubmit, control, setError, setValue, formState: { errors } } = useForm<JobFormValues>({
    resolver: zodResolver(schema), mode: "onChange",
    defaultValues: {
      name: "gpu-demo", image: "alpine:3.21", commandLines: "sh\n-c\nsleep 300", environmentLines: "",
      gpuCount: 1, minVramMiB: 1024, performanceProfile: "", fp8Required: false,
      cpuMilli: 1000, memoryMiB: 256,
      necessityReason: "", necessityExplanation: "", neededAt: "", ttlHours: 1,
    },
  });
  const values = useWatch({ control });
  const parsed = schema.safeParse(values);
  const inputKey = parsed.success ? JSON.stringify(allocationInput(parsed.data)) : "";
  const preview = useMutation({
    mutationFn: async (input: CreateJobInput) => ({
      inputKey: JSON.stringify(input), result: await api.previewJob(input),
    }),
  });
  const currentPreview = preview.data?.inputKey === inputKey ? preview.data.result : undefined;
  const busy = mutation.isPending || mutation.isSuccess || preview.isPending;
  const valid = Boolean(catalog.data && parsed.success);
  const profiles = catalog.data?.necessityProfiles.filter(p => p.workloadType === values.workloadType) ?? [];
  const selectedNecessity = profiles.find(p => p.level === values.necessityLevel);
  const reasons = selectedNecessity ? [...selectedNecessity.reasons, catalog.data!.customReason] : [];

  function reportError(error: Error) {
    if (error instanceof ApiError) {
      for (const [field, message] of Object.entries(error.fields)) {
        const formField = apiFields[field];
        if (formField) setError(formField, { type: "server", message });
      }
    }
    pushToast({ tone: "error", title: "Yêu cầu chưa được xử lý", description: error.message });
  }
  const evaluate = handleSubmit(form => preview.mutate(allocationInput(form), { onError: reportError }));
  const submit = handleSubmit(form => {
    if (!currentPreview || busy) return;
    mutation.mutate(allocationInput(form), {
      onSuccess: job => {
        pushToast({ tone: "success", title: "Đã đưa workload vào hàng đợi", description: job.policy.reason });
        router.push("/workloads/" + job.id);
      },
      onError: reportError,
    });
  });

  return <form onSubmit={submit} className="grid gap-5 xl:grid-cols-[1fr_360px]">
    <div className="space-y-5">
      {catalog.isPending && <p role="status">Đang tải lựa chọn từ hệ thống…</p>}
      {catalog.error && <div role="alert" className="field-error">Không tải được cấu hình: {catalog.error.message} <Button variant="secondary" onClick={() => catalog.refetch()}>Thử lại</Button></div>}
      <Card>
        <CardHeader title="Container" description="Thông tin workload cần triển khai" />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Tên workload" error={errors.name?.message}><input className="field-input" {...register("name")} /></Field>
          <Field label="Docker image" error={errors.image?.message}><input className="field-input" {...register("image")} /></Field>
          <Field label="Command — mỗi đối số một dòng" error={errors.commandLines?.message} wide><textarea rows={3} className="field-textarea font-mono" {...register("commandLines")} /></Field>
          <Field label="Environment — KEY=value mỗi dòng" error={errors.environmentLines?.message} wide><textarea rows={2} className="field-textarea font-mono" {...register("environmentLines")} /></Field>
        </div>
      </Card>
      <Card>
        <CardHeader title="Nhu cầu tài nguyên" description="Hệ thống tự chọn server và GPU phù hợp với yêu cầu." />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Loại workload" error={errors.workloadType?.message}><select className="field-select" {...register("workloadType", { onChange: () => setValue("necessityReason", "", { shouldValidate: true }) })}><Options items={catalog.data?.workloadTypes} /></select></Field>
          <Field label="Số GPU" error={errors.gpuCount?.message}><input type="number" min={1} step={1} max={catalog.data?.limits.maxGpuCount} className="field-input" {...register("gpuCount", { valueAsNumber: true })} /></Field>
          <Field label="VRAM tối thiểu / GPU (MiB)" error={errors.minVramMiB?.message}><input type="number" min={1} step={1} className="field-input" {...register("minVramMiB", { valueAsNumber: true })} /></Field>
          <Field label="Profile hiệu năng" error={errors.performanceProfile?.message}><select className="field-select" {...register("performanceProfile")}><Options items={catalog.data?.performanceProfiles} /></select></Field>
          <Field label="Yêu cầu FP8" error={errors.fp8Required?.message}><input type="checkbox" className="size-5" {...register("fp8Required")} /><span className="ml-2 text-sm">GPU phải hỗ trợ FP8</span></Field>
          <p className="text-sm text-slate-500">1024 MiB = 1 GiB. Cấp phát nguyên GPU trên cùng một server.</p>
          <Field label="CPU (millicores)" error={errors.cpuMilli?.message}><input type="number" min={0} className="field-input" {...register("cpuMilli", { setValueAs: optionalNumber })} /></Field>
          <Field label="RAM (MiB)" error={errors.memoryMiB?.message}><input type="number" min={0} className="field-input" {...register("memoryMiB", { setValueAs: optionalNumber })} /></Field>
        </div>
      </Card>
      <Card>
        <CardHeader title="Nhu cầu sử dụng" description="Chọn tính cần thiết và mức độ quan trọng để hệ thống đối chiếu chính sách." />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Tính cần thiết" error={errors.necessityLevel?.message}><select className="field-select" {...register("necessityLevel", { onChange: () => setValue("necessityReason", "", { shouldValidate: true }) })}><Options items={profiles.map(p => ({ id: p.level, label: p.label }))} /></select></Field>
          <Field label="Mức độ quan trọng hệ thống" error={errors.systemImportance?.message}><select className="field-select" {...register("systemImportance")}><Options items={catalog.data?.systemImportance} /></select></Field>
          <Field label="Lý do tính cần thiết" error={errors.necessityReason?.message} wide><select className="field-select" {...register("necessityReason")}><Options items={reasons} /></select></Field>
          {values.necessityReason === catalog.data?.customReason.id && <Field label="Giải trình tính cần thiết" error={errors.necessityExplanation?.message} wide><textarea rows={3} className="field-textarea" {...register("necessityExplanation")} /></Field>}
          <Field label="Thời điểm cần (giờ địa phương)" error={errors.neededAt?.message}><input type="datetime-local" className="field-input" {...register("neededAt")} /></Field>
          <Field label="Thời gian sử dụng (giờ)" error={errors.ttlHours?.message}><input type="number" min={1 / 60} step="any" max={catalog.data ? catalog.data.limits.maxTtlSeconds / 3600 : undefined} className="field-input" {...register("ttlHours", { valueAsNumber: true })} /></Field>
          <p className="text-sm text-slate-500 md:col-span-2">Thời điểm cần được chuyển sang UTC khi gửi; có thể khai báo nhu cầu đã quá hạn. Đây chưa phải lịch hẹn chạy. Thời lượng hiện là nhu cầu đăng ký, chưa tự dừng workload khi hết hạn.</p>
        </div>
      </Card>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link href="/workloads" className="btn btn-ghost"><ArrowLeft className="size-4" />Huỷ</Link>
        <Button variant="secondary" onClick={evaluate} disabled={!valid || busy}>{preview.isPending && <Loader2 className="size-4 animate-spin" />}Đối chiếu tự động</Button>
        <Button type="submit" disabled={!valid || !currentPreview || busy}>{mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Box className="size-4" />}Gửi vào hàng đợi</Button>
      </div>
      {(preview.error || mutation.error) && <p role="alert" className="field-error">{(preview.error ?? mutation.error)?.message}</p>}
    </div>
    <div className="xl:sticky xl:top-24 xl:self-start">
      <AllocationPreview result={currentPreview} stale={Boolean(preview.data && !currentPreview)} />
    </div>
  </form>;
}

function optionalNumber(value: string) { return value === "" ? undefined : Number(value); }
function Options({ items = [] }: { items?: AllocationOption[] }) {
  return <><option value="">Chọn…</option>{items.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</>;
}
function Field({ label, error, wide, children }: { label: string; error?: string; wide?: boolean; children: React.ReactNode }) {
  return <label className={wide ? "md:col-span-2" : undefined}><span className="field-label">{label}</span>{children}{error && <span className="field-error">{error}</span>}</label>;
}
