# Architecture

Tài liệu hợp nhất tại [workspace architecture](../../../docs/ARCHITECTURE.md), [audit ban đầu](../../../docs/ARCHITECTURE-AUDIT.md) và [Definition of Done tiếp nối](../../../docs/DEFINITION-OF-DONE.md).

Giữ bốn binaries trong cmd, Agent outbound HTTP, local Docker SDK và cùng NVML adapter cho real/sim. Domain/application không phụ thuộc Docker/NVML/HTTP; atomic commit/release trong store/memory, persistence snapshot ở store/durable.

Simulator chỉ thay DockerRuntime; GPU lấy từ NVIDIA nvml-mock shared library. Control Plane dùng snapshot durable version1 cho một process. PostgreSQL migrations là target tham khảo, không được thực thi. Không có Kubernetes adapter, MIG, GPU sharing hoặc preemption trong scope.

[README backend](../README.md) có source map/run/test; [README workspace](../../../README.md) có toàn bộ luồng demo UI/API/Agent.
