# Kiến trúc — tài liệu đã hợp nhất

Tài liệu canonical hiện tại là [SYSTEM_DESIGN.md](SYSTEM_DESIGN.md). Nội dung hữu ích về bốn composition roots, adapter boundaries, runtime snapshot/rollback, lifecycle, occupancy, observability và allocation đã được hợp nhất vào tài liệu đó cùng [DOMAIN_MODEL.md](DOMAIN_MODEL.md) và [ALGORITHMS.md](ALGORITHMS.md).

Giữ file này làm đường dẫn tương thích cho links cũ, không duy trì thêm một mô tả kiến trúc song song. PostgreSQL hiện lưu business metadata; runtime reservation/inventory vẫn dùng memory/durable. Hướng dẫn chạy và kiểm thử nằm ở [TESTING_RUNBOOK.md](TESTING_RUNBOOK.md).
