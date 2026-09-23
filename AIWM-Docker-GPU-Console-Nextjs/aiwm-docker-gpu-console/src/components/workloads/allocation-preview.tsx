import { Card, CardHeader } from "@/components/ui/card";
import type { JobPreview } from "@/lib/api/types";
import { formatDateTime } from "@/lib/utils/format";

export function AllocationPreview({ result, stale }: { result?: JobPreview; stale: boolean }) {
  return <Card aria-live="polite">
    <CardHeader title="ĐỐI CHIẾU TỰ ĐỘNG" description="Đối chiếu nhu cầu với chính sách và tài nguyên hiện tại." />
    <div className="space-y-4 p-5 text-sm">
      {!result ? <p>{stale ? "Thông tin đã thay đổi. Hãy đối chiếu lại trước khi gửi." : "Nhập đủ thông tin rồi chọn Đối chiếu tự động."}</p> : <>
        <p>{result.sizingPlanStatus}</p>
        <p>{result.quotaStatus}</p>
        <Detail label="Mức ưu tiên" value={result.necessityLabel} />
        <Detail label="Thời gian đăng ký" value={`${formatDateTime(result.requestedStartAt)} → ${formatDateTime(result.requestedEndAt)}`} />
        <Detail label="Lịch cấp phát" value={result.planningStatus === "AVAILABLE" ? "Có thể giữ tài nguyên trong khoảng thời gian này" : "Đang xung đột hoặc chưa đủ tài nguyên phù hợp"} />
        <p>{result.policyReason}</p>
        <Detail label="GPU phù hợp hiện tại" value={result.resourceMatch.recommendedGpuModels.join(" / ") || "Chưa có GPU phù hợp khả dụng"} />
        <Detail label="Phương án phù hợp" value={result.resourceMatch.matchedGpuCount + " GPU trong khoảng thời gian đăng ký"} />
        <Detail label="Khuyến nghị" value={result.resourceMatch.recommendationText} />
        {result.policySource === "DEVELOPMENT_CONFIG" && <p className="text-amber-700">Quy hoạch và hạn mức hiện lấy từ cấu hình demo.</p>}
        <p className="text-slate-500">Đối chiếu chưa giữ tài nguyên. Khi gửi, hệ thống kiểm tra lại; kết quả cấp phát có thể thay đổi.</p>
      </>}
    </div>
  </Card>;
}
function Detail({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs font-semibold text-slate-500">{label}</p><p className="mt-1 font-medium">{value}</p></div>;
}
