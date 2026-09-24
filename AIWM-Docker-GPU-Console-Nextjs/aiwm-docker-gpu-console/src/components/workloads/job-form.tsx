"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Box, Loader2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Children, cloneElement, isValidElement, useEffect, useId, useMemo, useState, type ReactElement } from "react";
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
  "resources.gpuCount": "gpuCount", "resources.minVramMiB": "minVramGB",
  "resources.performanceProfile": "performanceProfile", "resources.fp8Required": "fp8Required",
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
      gpuCount: 1, minVramGB: 1, performanceProfile: "AUTO", fp8Required: false,
      necessityReason: "", necessityExplanation: "", neededAt: "", ttlHours: 1,
    },
  });
  const values = useWatch({ control });
  const parsed = schema.safeParse(values);
  const inputKey = parsed.success ? JSON.stringify(allocationInput(parsed.data)) : "";
  const [debouncedKey, setDebouncedKey] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedKey(inputKey), 400);
    return () => clearTimeout(timer);
  }, [inputKey]);
  const valid = Boolean(catalog.data && parsed.success);
  const preview = useQuery({
    queryKey: ["allocation-preview", inputKey],
    queryFn: ({ signal }) => api.previewJob(JSON.parse(inputKey) as CreateJobInput, signal),
    enabled: valid && inputKey === debouncedKey,
    retry: false,
    staleTime: 0,
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });
  const currentPreview = inputKey === debouncedKey && preview.isSuccess && !preview.isFetching ? preview.data : undefined;
  const previewBusy = valid && (inputKey !== debouncedKey || preview.isFetching);
  const busy = mutation.isPending || mutation.isSuccess || previewBusy;
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
  const submit = handleSubmit(form => {
    if (!currentPreview || busy) return;
    mutation.mutate(allocationInput(form), {
      onSuccess: job => {
        pushToast({ tone: "success", title: "Đã đưa workload vào hàng đợi" });
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
          <Field label="Loại workload" error={errors.workloadType?.message}><select className="field-select" {...register("workloadType", { onChange: () => setValue("necessityReason", "", { shouldValidate: true }) })}><Options items={catalog.data?.workloadTypes} /></select></Field>
          <Field label="Tên workload" error={errors.name?.message}><input className="field-input" {...register("name")} /></Field>
          <Field label="Docker image" error={errors.image?.message}><input className="field-input" {...register("image")} /></Field>
          <Field label="Command — mỗi đối số một dòng" error={errors.commandLines?.message} wide><textarea rows={3} className="field-textarea font-mono" {...register("commandLines")} /></Field>
          <Field label="Environment — KEY=value mỗi dòng" error={errors.environmentLines?.message} wide><textarea rows={2} className="field-textarea font-mono" {...register("environmentLines")} /></Field>
        </div>
      </Card>
      <Card>
        <CardHeader title="Nhu cầu GPU" />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Số GPU" error={errors.gpuCount?.message}><input type="number" min={1} step={1} max={catalog.data?.limits.maxGpuCount} className="field-input" {...register("gpuCount", { valueAsNumber: true })} /></Field>
          <Field label="VRAM tối thiểu / GPU (GB)" error={errors.minVramGB?.message}><input type="number" min={0.25} step={0.25} className="field-input" {...register("minVramGB", { valueAsNumber: true })} /></Field>
          <Field label="Profile hiệu năng" error={errors.performanceProfile?.message}><select className="field-select" {...register("performanceProfile")}><Options items={catalog.data?.performanceProfiles} /></select></Field>
          <Field label="Yêu cầu FP8" error={errors.fp8Required?.message}><input type="checkbox" className="size-5" {...register("fp8Required")} /><span className="ml-2 text-sm">Có</span></Field>
        </div>
      </Card>
      <Card>
        <CardHeader title="Nhu cầu sử dụng" />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Tính cần thiết" error={errors.necessityLevel?.message}><select className="field-select" {...register("necessityLevel", { onChange: () => setValue("necessityReason", "", { shouldValidate: true }) })}><Options items={profiles.map(p => ({ id: p.level, label: p.label }))} /></select></Field>
          <Field label="Mức độ quan trọng hệ thống" error={errors.systemImportance?.message}><select className="field-select" {...register("systemImportance")}><Options items={catalog.data?.systemImportance} /></select></Field>
          <Field label="Lý do tính cần thiết" error={errors.necessityReason?.message} wide><select className="field-select" {...register("necessityReason")}><Options items={reasons} /></select></Field>
          {values.necessityReason === catalog.data?.customReason.id && <Field label="Giải trình tính cần thiết" error={errors.necessityExplanation?.message} wide><textarea rows={3} className="field-textarea" {...register("necessityExplanation")} /></Field>}
          <Field label="Thời điểm bắt đầu mong muốn (giờ địa phương)" error={errors.neededAt?.message}><input type="datetime-local" className="field-input" {...register("neededAt")} /></Field>
          <Field label="Thời lượng sử dụng (giờ)" error={errors.ttlHours?.message}><input type="number" list="duration-presets" min={1 / 60} step="any" max={catalog.data ? catalog.data.limits.maxTtlSeconds / 3600 : undefined} className="field-input" {...register("ttlHours", { valueAsNumber: true })} /></Field>
          <datalist id="duration-presets">{[0.5, 1, 2, 4, 8].map(hours => <option key={hours} value={hours}>{hours === 0.5 ? "30 phút" : hours + " giờ"}</option>)}</datalist>
        </div>
      </Card>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link href="/workloads" className="btn btn-ghost"><ArrowLeft className="size-4" />Huỷ</Link>
        <Button type="submit" disabled={!valid || !currentPreview || busy}>{mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Box className="size-4" />}Gửi vào hàng đợi</Button>
      </div>
      {(preview.error || mutation.error) && <p role="alert" className="field-error">{(preview.error ?? mutation.error)?.message}</p>}
    </div>
    <div className="xl:sticky xl:top-24 xl:self-start">
      <AllocationPreview result={currentPreview} loading={previewBusy} error={preview.isError ? preview.error.message : undefined} onRetry={() => void preview.refetch()} />
    </div>
  </form>;
}

function Options({ items = [] }: { items?: AllocationOption[] }) {
  return <><option value="">Chọn…</option>{items.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</>;
}
function Field({ label, error, wide, children }: { label: string; error?: string; wide?: boolean; children: React.ReactNode }) {
  const id = useId();
  return <div className={wide ? "md:col-span-2" : undefined}>
    <label htmlFor={id} className="field-label">{label}</label>
    {Children.map(children, child => isValidElement(child) && ["input", "select", "textarea"].includes(String(child.type))
      ? cloneElement(child as ReactElement<{ id?: string; "aria-describedby"?: string; "aria-invalid"?: boolean }>, { id, "aria-describedby": error ? id + "-error" : undefined, "aria-invalid": Boolean(error) })
      : child)}
    {error && <p id={id + "-error"} role="alert" className="field-error">{error}</p>}
  </div>;
}
