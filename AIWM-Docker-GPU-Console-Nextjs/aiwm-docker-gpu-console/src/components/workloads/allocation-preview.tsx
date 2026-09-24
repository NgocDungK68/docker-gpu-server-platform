import { CheckCircle2, Loader2 } from "lucide-react";
import { Card, CardHeader } from "@/components/ui/card";
import type { JobPreview } from "@/lib/api/types";
import { formatDateTime } from "@/lib/utils/format";

export function AllocationPreview({ result, loading, error, onRetry }: {
  result?: JobPreview; loading: boolean; error?: string; onRetry: () => void;
}) {
  return <Card aria-live="polite">
    <CardHeader title="Tài nguyên phù hợp" />
    <div className="space-y-4 p-5 text-sm">
      {loading ? <p role="status" className="flex items-center gap-2"><Loader2 className="size-4 animate-spin" />Đang kiểm tra tài nguyên…</p>
        : error ? <div role="alert"><p className="text-red-700">{error}</p><button type="button" className="mt-2 font-semibold underline" onClick={onRetry}>Thử lại</button></div>
        : !result ? <p className="text-slate-500">Nhập đủ thông tin để xem tài nguyên phù hợp.</p>
        : <>
          <p className={result.resourceMatch.satisfiable ? "flex items-center gap-2 font-semibold text-emerald-700" : "font-semibold text-amber-700"}>
            {result.resourceMatch.satisfiable && <CheckCircle2 className="size-5" />}
            {result.resourceMatch.satisfiable ? "Có tài nguyên phù hợp" : "Không đủ GPU trong khoảng thời gian đã chọn."}
          </p>
          <Detail label="Thời gian sử dụng" value={formatDateTime(result.requestedStartAt) + " → " + formatDateTime(result.requestedEndAt)} />
          {result.resourceMatch.satisfiable && <>
            <Detail label="GPU phù hợp" value={result.resourceMatch.recommendedGpuModels.join(", ")} />
            <Detail label="Có thể đáp ứng" value={result.resourceMatch.matchedGpuCount + " GPU theo yêu cầu"} />
          </>}
          {!result.resourceMatch.satisfiable && <p>Thử giảm số GPU, đổi yêu cầu hoặc chọn khoảng thời gian khác.</p>}
        </>}
    </div>
  </Card>;
}
function Detail({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs font-semibold text-slate-500">{label}</p><p className="mt-1 font-medium">{value}</p></div>;
}
