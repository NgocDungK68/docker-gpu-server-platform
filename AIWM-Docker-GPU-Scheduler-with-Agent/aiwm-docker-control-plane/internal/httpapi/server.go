package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type Server struct {
	controlPlane *application.ControlPlane
	logger       *slog.Logger
	handler      http.Handler
}

// Options defines public API credentials separately from per-agent authentication.
type Options struct {
	PublicAPIToken string // Chỉ tương thích test harness cũ; production dùng Identity.
	Identity *application.IdentityService
}

func New(controlPlane *application.ControlPlane, logger *slog.Logger, corsOrigins []string, options ...Options) *Server {
	server := &Server{controlPlane: controlPlane, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /api/v1/system/summary", server.summary)

	mux.HandleFunc("POST /api/v1/agents/register", server.registerAgent)
	mux.HandleFunc("POST /api/v1/agents/{agentID}/heartbeat", server.withAgentAuth(server.heartbeat))
	mux.HandleFunc("PUT /api/v1/agents/{agentID}/inventory", server.withAgentAuth(server.inventory))
	mux.HandleFunc("GET /api/v1/agents/{agentID}/commands", server.withAgentAuth(server.pollCommands))
	mux.HandleFunc("POST /api/v1/agents/{agentID}/commands/{commandID}/ack", server.withAgentAuth(server.ackCommand))

	mux.HandleFunc("GET /api/v1/servers", server.listServers)
	mux.HandleFunc("GET /api/v1/servers/{serverID}", server.getServer)
	mux.HandleFunc("POST /api/v1/servers/{serverID}/drain", server.drainServer)
	mux.HandleFunc("GET /api/v1/gpus", server.listGPUs)
	mux.HandleFunc("GET /api/v1/containers", server.listContainers)

	mux.HandleFunc("POST /api/v1/jobs", server.createJob)
	mux.HandleFunc("POST /api/v1/jobs/preview", server.previewJob)
	mux.HandleFunc("GET /api/v1/jobs/options", server.jobOptions)
	mux.HandleFunc("GET /api/v1/jobs", server.listJobs)
	mux.HandleFunc("GET /api/v1/jobs/{jobID}", server.getJob)
	mux.HandleFunc("POST /api/v1/jobs/{jobID}/stop", server.stopJob)
	mux.HandleFunc("GET /api/v1/queue", server.queue)
	mux.HandleFunc("POST /api/v1/scheduler/run-once", server.runScheduler)

	var handler http.Handler = mux
	if len(options) > 0 {
		if options[0].Identity != nil {
			mountIdentity(mux,options[0].Identity)
			handler = sessionAuth(options[0].Identity,handler)
		} else {
			handler = publicAuth(options[0].PublicAPIToken, handler)
		}
	}
	handler = bodyLimit(handler)
	handler = cors(corsOrigins, handler)
	handler = accessLog(logger, handler)
	handler = recoverPanic(logger, handler)
	handler = requestContext(handler)
	server.handler = handler
	return server
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, domain.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (s *Server) ready(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, domain.APIResponse{Data: map[string]string{"status": "ready"}})
}

func (s *Server) summary(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.Summary(request.Context())
	for i := range result.RecentEvents {
		result.RecentEvents[i].Reason = publicReason(result.RecentEvents[i].Reason)
	}
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) registerAgent(writer http.ResponseWriter, request *http.Request) {
	var input agentv1.RegisterRequest
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.RegisterAgent(request.Context(), request.Header.Get(agentv1.EnrollmentTokenHeader), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) heartbeat(writer http.ResponseWriter, request *http.Request) {
	var input agentv1.HeartbeatRequest
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.Heartbeat(request.Context(), request.PathValue("agentID"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) inventory(writer http.ResponseWriter, request *http.Request) {
	var input agentv1.InventoryReport
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.ReportInventory(request.Context(), request.PathValue("agentID"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) pollCommands(writer http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	result, err := s.controlPlane.PollCommands(request.Context(), request.PathValue("agentID"), limit)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) ackCommand(writer http.ResponseWriter, request *http.Request) {
	var input agentv1.CommandAckRequest
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.AckCommand(request.Context(), request.PathValue("agentID"), request.PathValue("commandID"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) listServers(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.ListServers(request.Context())
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) getServer(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.GetServer(request.Context(), request.PathValue("serverID"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) drainServer(writer http.ResponseWriter, request *http.Request) {
	var input domain.DrainServerRequest
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.SetServerDrained(request.Context(), request.PathValue("serverID"), input.Drained)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) listGPUs(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.GPUs(request.Context())
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) listContainers(writer http.ResponseWriter, request *http.Request) {
	origin := domain.ContainerOrigin(strings.ToUpper(request.URL.Query().Get("origin")))
	result, err := s.controlPlane.Containers(request.Context(), origin)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) jobOptions(w http.ResponseWriter, r *http.Request) {
	respond(w, s.controlPlane.AllocationOptions(), nil, http.StatusOK)
}
func (s *Server) previewJob(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateJobRequest
	if err := decodeJSON(r, &input); err != nil {
		respond(w, nil, err, 0)
		return
	}
	result, err := s.controlPlane.PreviewJob(r.Context(), input)
	respond(w, result, err, http.StatusOK)
}
func (s *Server) createJob(writer http.ResponseWriter, request *http.Request) {
	var input domain.CreateJobRequest
	if err := decodeJSON(request, &input); err != nil {
		respond(writer, nil, err, 0)
		return
	}
	result, err := s.controlPlane.CreateJob(request.Context(), input)
	respond(writer, publicJob(result), err, http.StatusCreated)
}

func (s *Server) listJobs(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.ListJobs(request.Context())
	respond(writer, publicJobs(result), err, http.StatusOK)
}

func (s *Server) getJob(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.GetJob(request.Context(), request.PathValue("jobID"))
	respond(writer, publicJob(result), err, http.StatusOK)
}

func (s *Server) stopJob(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.StopJob(request.Context(), request.PathValue("jobID"))
	respond(writer, publicJob(result), err, http.StatusAccepted)
}

func (s *Server) queue(writer http.ResponseWriter, request *http.Request) {
	result, err := s.controlPlane.Queue(request.Context())
	respond(writer, publicJobs(result), err, http.StatusOK)
}

func (s *Server) runScheduler(writer http.ResponseWriter, request *http.Request) {
	count, err := s.controlPlane.ScheduleOnce(request.Context())
	respond(writer, map[string]int{"assignedJobs": count}, err, http.StatusOK)
}

func (s *Server) withAgentAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		token := bearerToken(request.Header.Get("Authorization"))
		if err := s.controlPlane.AuthenticateAgent(request.Context(), request.PathValue("agentID"), token); err != nil {
			respond(writer, nil, domain.ErrUnauthorized, 0)
			return
		}
		next(writer, request)
	}
}

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return err
		}
		var typeError *json.UnmarshalTypeError
		field := "body"
		if errors.As(err, &typeError) && typeError.Field != "" {
			field = typeError.Field
		}
		return &domain.ValidationError{Fields: map[string]string{field: "Sai kiểu dữ liệu, thiếu giá trị hoặc có field không được hỗ trợ"}}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.Join(domain.ErrInvalidInput, errors.New("request body must contain one JSON value"))
	}
	return nil
}

func respond(writer http.ResponseWriter, data any, err error, successStatus int) {
	if err == nil {
		writeJSON(writer, successStatus, domain.APIResponse{Data: data})
		return
	}
	status, code := http.StatusInternalServerError, "INTERNAL"
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "INVALID_INPUT"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, domain.ErrForbidden):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrStaleInventory), errors.Is(err, domain.ErrJobNotStoppable):
		status, code = http.StatusConflict, "CONFLICT"
	case errors.Is(err, domain.ErrInsufficientGPU):
		status, code = http.StatusUnprocessableEntity, "INSUFFICIENT_GPU"
	}
	message := err.Error()
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		status, code, message = http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body exceeds 1 MiB"
	}
	if status == http.StatusInternalServerError {
		slog.Error("API operation failed", "error", err)
		message = "internal server error"
	}
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		writeJSON(writer, status, domain.APIResponse{Error: &domain.APIError{Code: code, Message: message, Fields: validation.Fields}})
		return
	}
	writeError(writer, status, code, message)
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, domain.APIResponse{Error: &domain.APIError{Code: code, Message: message}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		slog.Error("write response", "error", err)
	}
}
