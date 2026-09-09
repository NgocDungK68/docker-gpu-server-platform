import { PageHeader } from "@/components/ui/page";
import { JobForm } from "@/components/workloads/job-form";

export default function NewWorkloadPage() {
  return <><PageHeader eyebrow="Submit job" title="Tạo Docker workload" description="Khai báo yêu cầu tài nguyên; scheduler sẽ chọn server và GPU UUID phù hợp trong logical pool." /><JobForm /></>;
}
