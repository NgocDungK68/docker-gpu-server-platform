export const statusLabels: Record<string, string> = {
  FREE: "Sẵn sàng", RESERVED: "Đã giữ tài nguyên", ALLOCATED: "Đang sử dụng",
  OCCUPIED_LEGACY: "Đang dùng ngoài hệ thống", OCCUPIED_UNKNOWN: "Chưa xác định", UNHEALTHY: "Không khả dụng",
  ONLINE: "Trực tuyến", OFFLINE: "Mất kết nối", DRAINING: "Ngừng nhận workload mới",
  QUEUED: "Đang chờ", ASSIGNED: "Đã lên lịch", STARTING: "Đang khởi động", RUNNING: "Đang chạy",
  STOPPING: "Đang dừng", STOPPED: "Đã dừng", CANCELLED: "Đã hủy", SUCCEEDED: "Hoàn thành", FAILED: "Thất bại",
  PLANNED: "Đã lên lịch", RELEASED: "Đã trả tài nguyên",
  MANAGED: "Do AIWM quản lý", LEGACY: "Có sẵn trên máy", UNKNOWN: "Chưa xác định",
  CREATED: "Đã tạo", EXITED: "Đã kết thúc", DEAD: "Đã kết thúc", PAUSED: "Tạm dừng", RESTARTING: "Đang khởi động lại",
};
export const profileLabel = (profile?: string) => profile === "HIGH_PERFORMANCE" ? "Hiệu năng cao" : profile === "AUTO" ? "Auto" : profile || "Auto";

// Backend diagnostic strings remain in logs/API; present actionable summaries here.
export function workloadMessage(status: string, reason = ""): string {
  if (/expir|hết (hạn|thời)|kết thúc khoảng/i.test(reason)) return "Đã hết thời gian sử dụng đã đăng ký.";
  if (/image|pull/i.test(reason) && status === "FAILED") return "Không tải được image. Kiểm tra tên image và quyền truy cập.";
  if (status === "QUEUED") return "Chưa đủ tài nguyên phù hợp trong khoảng thời gian đã chọn.";
  if (status === "ASSIGNED") return "Đã giữ tài nguyên, đang chờ đến thời gian chạy.";
  if (status === "FAILED") return "Không thể chạy workload. Kiểm tra cấu hình hoặc liên hệ quản trị viên.";
  return "";
}
