import { z } from "zod";
import type { AllocationOptions, CreateJobInput } from "../api/types";

export function parseKeyValueLines(value: string): Record<string, string> {
  return Object.fromEntries(value.split("\n").filter((line) => line.trim()).map((line) => {
    const index = line.indexOf("=");
    if (index < 1) throw new Error("Mỗi dòng cần có dạng KEY=value");
    return [line.slice(0, index).trim(), line.slice(index + 1)];
  }));
}
function keyValueLines(environment: boolean) {
  return z.string().superRefine((text, context) => {
    const keys = new Set<string>();
    for (const line of text.split("\n").filter((item) => item.trim())) {
      const index = line.indexOf("=");
      const key = line.slice(0, index).trim();
      if (index < 1 || keys.has(key) || (environment && (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || key.startsWith("NVIDIA_") || key === "CUDA_VISIBLE_DEVICES"))) {
        context.addIssue({ code: "custom", message: "Biến môi trường không hợp lệ, bị trùng hoặc sử dụng tên dành riêng cho hệ thống." });
      }
      keys.add(key);
    }
  });
}
// Reject impossible local dates before converting them to UTC for the API.
function validLocalDateTime(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$/.test(value)) return false;
  const [year, month, day, hour, minute, second = 0] = value.split(/[-T:]/).map(Number);
  const date = new Date(value);
  return Number.isFinite(date.getTime()) && date.getFullYear() === year && date.getMonth() + 1 === month
    && date.getDate() === day && date.getHours() === hour && date.getMinutes() === minute && date.getSeconds() === second;
}
export const jobFormSchema = z.object({
  name: z.string().trim().min(3, "Tên cần ít nhất 3 ký tự").max(128),
  image: z.string().trim().min(1, "Cần nhập image").max(512).regex(/^[^\s\u0000]+$/, "Image không được chứa khoảng trắng"),
  commandLines: z.string().max(65536),
  environmentLines: keyValueLines(true),
  gpuCount: z.number().int("Số GPU phải là số nguyên").positive("Số GPU phải lớn hơn 0"),
  performanceProfile: z.enum(["AUTO", "HIGH_PERFORMANCE"]),
  fp8Required: z.boolean(),
  minVramGB: z.number().finite().positive("VRAM phải lớn hơn 0 GB").refine(value => Number.isSafeInteger(value * 1024), "VRAM phải quy đổi được thành số MiB nguyên"),
  workloadType: z.enum(["TRAINING", "INFERENCE"], { errorMap: () => ({ message: "Chọn loại workload" }) }),
  necessityLevel: z.enum(["NECESSITY_1", "NECESSITY_2", "NECESSITY_3", "NECESSITY_4"], { errorMap: () => ({ message: "Chọn tính cần thiết" }) }),
  necessityReason: z.string().min(1, "Chọn lý do"),
  necessityExplanation: z.string().max(4096),
  systemImportance: z.enum(["CRITICAL_SPECIAL", "VERY_IMPORTANT", "IMPORTANT", "NORMAL"], { errorMap: () => ({ message: "Chọn mức độ quan trọng" }) }),
  neededAt: z.string().refine(validLocalDateTime, "Chọn ngày và giờ hợp lệ theo giờ địa phương"),
  ttlHours: z.number().positive("Thời lượng phải lớn hơn 0").refine(value => Number.isSafeInteger(value * 3600), "Thời lượng phải quy đổi được thành số giây nguyên"),
}).strict().superRefine((value, ctx) => {
  if (validLocalDateTime(value.neededAt) && new Date(value.neededAt).getTime() < Date.now() - 60_000) {
    ctx.addIssue({ code: "custom", path: ["neededAt"], message: "Chọn thời điểm bắt đầu từ hiện tại hoặc trong tương lai" });
  }
  if (value.necessityReason === "CUSTOM" && !value.necessityExplanation.trim()) {
    ctx.addIssue({ code: "custom", path: ["necessityExplanation"], message: "Nhập giải trình cho lý do khác" });
  }
});
export type JobFormValues = z.infer<typeof jobFormSchema>;

// Limits, supported profiles and reason choices come from the backend catalog.
export function allocationFormSchema(options?: AllocationOptions) {
  return jobFormSchema.superRefine((value, ctx) => {
    if (!options) return;
    if (value.gpuCount > options.limits.maxGpuCount) ctx.addIssue({code:"custom",path:["gpuCount"],message:"Tối đa " + options.limits.maxGpuCount + " GPU"});
    if (value.ttlHours * 3600 > options.limits.maxTtlSeconds) ctx.addIssue({code:"custom",path:["ttlHours"],message:"Tối đa " + options.limits.maxTtlSeconds / 3600 + " giờ"});
    if (!options.performanceProfiles.some(p => p.id === value.performanceProfile)) ctx.addIssue({code:"custom",path:["performanceProfile"],message:"Profile không được hỗ trợ"});
    const profile = options.necessityProfiles.find(p => p.workloadType === value.workloadType && p.level === value.necessityLevel);
    if (value.necessityReason !== options.customReason.id && !profile?.reasons.some(r => r.id === value.necessityReason)) ctx.addIssue({code:"custom",path:["necessityReason"],message:"Lý do không phù hợp workload và tính cần thiết"});
  });
}

// Convert local browser time to explicit UTC and hours to seconds; no policy calculation.
export function allocationInput(form: JobFormValues): CreateJobInput {
  return {
    name: form.name.trim(), image: form.image.trim(), backend: "DOCKER",
    command: form.commandLines.split("\n").map(item => item.trim()).filter(Boolean),
    environment: parseKeyValueLines(form.environmentLines),
    resources: {
      gpuCount: form.gpuCount, minVramMiB: form.minVramGB * 1024,
      performanceProfile: form.performanceProfile, fp8Required: form.fp8Required,
    },
    workloadType:form.workloadType,necessityLevel:form.necessityLevel,necessityReason:form.necessityReason,
    necessityExplanation:form.necessityExplanation.trim() || undefined,systemImportance:form.systemImportance,
    neededAt:new Date(form.neededAt).toISOString(),ttlSeconds:form.ttlHours*3600,
  };
}
