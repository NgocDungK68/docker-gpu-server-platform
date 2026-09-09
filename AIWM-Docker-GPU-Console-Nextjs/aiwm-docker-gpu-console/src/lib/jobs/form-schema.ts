import { z } from "zod";
import type { AllocationOptions, CreateJobInput } from "../api/types";

export function parseKeyValueLines(value: string): Record<string, string> {
  return Object.fromEntries(value.split("\n").filter((line) => line.trim()).map((line) => {
    const index = line.indexOf("=");
    if (index < 1) throw new Error("Má»—i dÃ²ng cáº§n KEY=value");
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
        context.addIssue({ code: "custom", message: "Cáº§n KEY=value há»£p lá»‡, khÃ´ng trÃ¹ng key; GPU visibility do Agent quáº£n lÃ½." });
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
const optionalNumber = z.number().int().nonnegative().optional();
export const jobFormSchema = z.object({
  name: z.string().trim().min(3, "TÃªn cáº§n Ã­t nháº¥t 3 kÃ½ tá»±").max(128),
  image: z.string().trim().min(1, "Cáº§n nháº­p Docker image").max(512).regex(/^[^\s\u0000]+$/, "Image khÃ´ng Ä‘Æ°á»£c chá»©a khoáº£ng tráº¯ng"),
  commandLines: z.string().max(65536),
  environmentLines: keyValueLines(true),
  gpuCount: z.number().int("Số GPU phải là số nguyên").positive("Số GPU phải lớn hơn 0"),
  performanceProfile: z.string().min(1, "Chọn profile hiệu năng"),
  fp8Required: z.boolean(),
  minVramMiB: z.number().int().positive("VRAM phải lớn hơn 0 MiB"),
  cpuMilli: optionalNumber,
  memoryMiB: optionalNumber,
  workloadType: z.enum(["TRAINING", "INFERENCE"], { errorMap: () => ({ message: "Chọn loại workload" }) }),
  necessityLevel: z.enum(["NECESSITY_1", "NECESSITY_2", "NECESSITY_3", "NECESSITY_4"], { errorMap: () => ({ message: "Chọn tính cần thiết" }) }),
  necessityReason: z.string().min(1, "Chọn lý do"),
  necessityExplanation: z.string().max(4096),
  systemImportance: z.enum(["CRITICAL_SPECIAL", "VERY_IMPORTANT", "IMPORTANT", "NORMAL"], { errorMap: () => ({ message: "Chọn mức độ quan trọng" }) }),
  neededAt: z.string().refine(validLocalDateTime, "Chọn ngày và giờ hợp lệ theo giờ địa phương"),
  ttlHours: z.number().positive("Thời lượng phải lớn hơn 0").refine(value => Number.isSafeInteger(value * 3600), "Thời lượng phải quy đổi được thành số giây nguyên"),
}).strict().superRefine((value, ctx) => {
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
      gpuCount: form.gpuCount, minVramMiB: form.minVramMiB,
      performanceProfile: form.performanceProfile, fp8Required: form.fp8Required,
      cpuMilli: form.cpuMilli, memoryMiB: form.memoryMiB,
    },
    workloadType:form.workloadType,necessityLevel:form.necessityLevel,necessityReason:form.necessityReason,
    necessityExplanation:form.necessityExplanation.trim() || undefined,systemImportance:form.systemImportance,
    neededAt:new Date(form.neededAt).toISOString(),ttlSeconds:form.ttlHours*3600,
  };
}
