# Phạm vi MVP và bước tiếp theo

Trạng thái thực hiện đã chuyển sang [Definition of Done](../../../docs/DEFINITION-OF-DONE.md) và [Scope traceability](../../../docs/TRACEABILITY.md); không dùng timeline cũ làm cam kết chức năng hiện tại.

Đã có shared Agent core, NVIDIA mock library, 4 policies, atomic reserve/release, durable restart, protected External inventory, UI/API thật, auth/validation và tests.

Các bước production tiếp theo có căn cứ: real-GPU acceptance, PostgreSQL transactional adapter/HA nếu cần nhiều CP, snapshot retention/compaction, user login/RBAC/audit, registry authentication và image policy. Không đưa Kubernetes/MIG/time-sharing/checkpoint/preemption vào roadmap MVP.
