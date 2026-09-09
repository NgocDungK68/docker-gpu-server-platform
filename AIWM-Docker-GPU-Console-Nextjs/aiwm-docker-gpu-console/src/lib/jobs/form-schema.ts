import { z } from "zod";

export function parseKeyValueLines(value: string): Record<string, string> {
  return Object.fromEntries(value.split("\n").filter((line) => line.trim()).map((line) => {
    const index = line.indexOf("=");
    if (index < 1) throw new Error("Mỗi dòng cần KEY=value");
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
        context.addIssue({ code: "custom", message: "Cần KEY=value hợp lệ, không trùng key; GPU visibility do Agent quản lý." });
      }
      keys.add(key);
    }
  });
}
const optionalNumber = z.number().int().nonnegative().optional();
export const jobFormSchema = z.object({
  name: z.string().trim().min(3, "Tên cần ít nhất 3 ký tự").max(128),
  image: z.string().trim().min(1, "Cần nhập Docker image").max(512).regex(/^[^\s\u0000]+$/, "Image không được chứa khoảng trắng"),
  commandLines: z.string().max(65536),
  environmentLines: keyValueLines(true),
  gpuCount: z.number().int().min(1).max(64),
  gpuModel: z.string(),
  minVramMiB: optionalNumber,
  cpuMilli: optionalNumber,
  memoryMiB: optionalNumber,
  priority: z.number().int().min(0).max(1000),
  strategy: z.enum(["first-fit", "best-fit", "bin-pack", "fragmentation-aware"]),
  selectorLines: keyValueLines(false),
});
export type JobFormValues = z.infer<typeof jobFormSchema>;
