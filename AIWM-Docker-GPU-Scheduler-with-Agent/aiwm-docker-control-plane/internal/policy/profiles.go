// Package policy evaluates business admission and queue order, never GPU placement.
package policy

import "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"

type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type NecessityProfile struct {
	WorkloadType domain.WorkloadType   `json:"workloadType"`
	Level        domain.NecessityLevel `json:"level"`
	Label        string                `json:"label"`
	Reasons      []Option              `json:"reasons"`
}

func Profiles() []NecessityProfile {
	return []NecessityProfile{
		{domain.WorkloadTraining, domain.Necessity1, "Cần thiết 1", []Option{{"EXECUTIVE_DIRECTION", "Chỉ đạo của Ban TGĐ Tập đoàn"}, {"CONTRACT_PENALTY", "Cam kết hợp đồng có phạt"}, {"INCIDENT_RETRAINING", "Sự cố cần huấn luyện lại khẩn"}, {"LEGAL_DEADLINE", "Yêu cầu pháp lý sắp đến hạn"}}},
		{domain.WorkloadTraining, domain.Necessity2, "Cần thiết 2", []Option{{"GO_LIVE_90_DAYS", "Dự án go-live dưới 90 ngày"}, {"BUSINESS_VALUE", "Doanh thu hoặc tiết kiệm lớn"}, {"APPROVED_PLAN", "Kế hoạch thực hiện hoặc phương án kinh doanh đã phê duyệt"}}},
		{domain.WorkloadTraining, domain.Necessity3, "Cần thiết 3", []Option{{"PERIODIC_RETRAINING", "Huấn luyện lại định kỳ"}, {"INTERNAL_IMPROVEMENT", "Cải tiến nội bộ"}}},
		{domain.WorkloadTraining, domain.Necessity4, "Cần thiết 4", []Option{{"EARLY_RESEARCH", "Nghiên cứu ban đầu"}, {"MODEL_EXPERIMENT", "Thử nghiệm mô hình mới"}}},
		{domain.WorkloadInference, domain.Necessity1, "Cần thiết 1", []Option{{"EXECUTIVE_DIRECTION", "Chỉ đạo của Ban TGĐ Tập đoàn"}, {"BROAD_EMERGENCY", "Khẩn cấp, ảnh hưởng rộng"}, {"INCIDENT_MITIGATION", "Khắc phục sự cố"}, {"LEGAL_REQUIREMENT", "Yêu cầu pháp lý bắt buộc"}}},
		{domain.WorkloadInference, domain.Necessity2, "Cần thiết 2", []Option{{"CUSTOMER_SLA", "Sản phẩm phục vụ khách hàng có SLA chặt"}}},
		{domain.WorkloadInference, domain.Necessity3, "Cần thiết 3", []Option{{"INTERNAL_PROCESS", "Quy trình nội bộ"}, {"BATCH_PROCESSING", "Xử lý theo lô"}, {"NON_REALTIME", "Không ràng buộc độ trễ tức thời"}}},
		{domain.WorkloadInference, domain.Necessity4, "Cần thiết 4", []Option{{"RESEARCH", "R&D"}, {"EXPERIMENT", "Thử nghiệm"}, {"DEMO", "Demo"}, {"DEV_UAT", "Dev/UAT"}}},
	}
}
func ImportanceOptions() []Option {
	return []Option{{string(domain.ImportanceCritical), "Đặc biệt quan trọng"}, {string(domain.ImportanceVeryImportant), "Rất quan trọng"}, {string(domain.ImportanceImportant), "Quan trọng"}, {string(domain.ImportanceNormal), "Bình thường"}}
}
func NecessityLabel(level domain.NecessityLevel) string {
	for _, profile := range Profiles() {
		if profile.Level == level {
			return profile.Label
		}
	}
	return "Yêu cầu cũ chưa khai báo tính cần thiết"
}
