# Agent protocol v1

Wire DTO độc lập ở internal/agentprotocol/v1/types.go. [OpenAPI](../api/openapi.yaml) mô tả endpoints; [security](../../../docs/SECURITY.md) mô tả credential.

1. POST /api/v1/agents/register: X-Enrollment-Token, protocolVersion=v1, machineId/name, optional labels/address/version/capabilities. Response data có agentId, agentToken, heartbeatIntervalSeconds.
2. POST /agents/{agentID}/heartbeat: Bearer token của đúng Agent, observedAt; CP dùng receipt time cho connectivity.
3. PUT /agents/{agentID}/inventory: full snapshot sequence tăng, observedAt, host, dockerVersion, GPUs, containers, processes. CP validate và normalize authoritative state; sequence cũ HTTP409.
4. GET /agents/{agentID}/commands?limit=10: lease START_CONTAINER/STOP_CONTAINER, payload DTO. Redelivery khi lease hết hạn.
5. POST /agents/{agentID}/commands/{commandID}/ack: succeeded/message/containerId. Command phải thuộc Agent; ACK đầu tiên quyết định, ACK lặp không làm lùi lifecycle.

Các path bước2–5 có prefix /api/v1. Public/operator API token không phải Agent token. Frontend không gọi những endpoints này.

Agent lưu result trước ACK. Nếu crash giữa Docker operation và lưu result, ownership labels/job ID dùng để tìm container đã có; không restart exited managed container. ACK start chỉ xác nhận command, inventory actual xác nhận RUNNING; stop chờ actual exit/missing trước release.

Không heartbeat/CP không kéo theo stop command. Agent retry registration/connectivity có context cancellation, inventory/events/poll loops độc lập. Machine ID/state không được dùng đồng thời bởi hai Agent.

Inventory wire origin/state chỉ là observation. CP giữ Container.Origin sau normalize literal; không rewrite MANAGED thành LEGACY khi Job không còn được biết. GPU accounting chỉ công nhận ALLOCATED khi active MANAGED container khớp known Job/Assignment cả Server và UUID; grant không khớp được tính OCCUPIED_LEGACY. Job reconciliation dùng JobID/assigned Server, chưa kiểm tra exact UUID như accounting. Process/container GPU mapping không chắc chắn không được xem free. AgentID trong protocol là Server.ID, không có AgentStatus riêng. Xem [Domain Model](../../../docs/DOMAIN_MODEL.md) để phân biệt các state/ownership và giới hạn reconciliation.
