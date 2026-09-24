package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func (s *Server) mountTraining(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/training/{jobID}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		contract, err := s.controlPlane.TrainingContract(r.Context(), r.PathValue("jobID"), bearerToken(r.Header.Get("Authorization")))
		respond(w, contract, err, 200)
	})
	mux.HandleFunc("PUT /api/v1/training/{jobID}/{kind}", func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if _, err := s.controlPlane.TrainingSession(r.Context(), r.PathValue("jobID"), token); err != nil {
			respond(w, nil, err, 0)
			return
		}
		maxBytes, timeout := s.controlPlane.TrainingUploadLimits()
		if r.ContentLength <= 0 || r.ContentLength > maxBytes {
			writeError(w, 413, "OUTPUT_SIZE", "Tệp phải có Content-Length hợp lệ và nằm trong giới hạn dung lượng")
			return
		}
		step := int64(0)
		if raw := r.URL.Query().Get("step"); raw != "" {
			var err error
			step, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || step < 0 {
				respond(w, nil, domain.ErrInvalidInput, 0)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		// Only authenticated binary uploads receive longer I/O deadlines.
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(timeout))
		_ = controller.SetWriteDeadline(time.Now().Add(timeout + 5*time.Second))
		result, err := s.controlPlane.SaveTrainingOutput(r.Context(), r.PathValue("jobID"), token, r.PathValue("kind"), step, r.ContentLength, r.Body)
		w.Header().Set("Cache-Control", "no-store")
		respond(w, result, err, 200)
	})
	mux.HandleFunc("POST /api/v1/jobs/{jobID}/continue", func(w http.ResponseWriter, r *http.Request) {
		var input application.ContinueTrainingRequest
		if err := decodeJSON(r, &input); err != nil {
			respond(w, nil, err, 0)
			return
		}
		job, err := s.controlPlane.ContinueTraining(r.Context(), r.PathValue("jobID"), input)
		respond(w, publicJob(job), err, 201)
	})
	mux.HandleFunc("GET /api/v1/jobs/{jobID}/artifact", func(w http.ResponseWriter, r *http.Request) {
		uri, err := s.controlPlane.ArtifactDownload(r.Context(), r.PathValue("jobID"))
		w.Header().Set("Cache-Control", "no-store")
		respond(w, map[string]any{"url": uri, "expiresInSeconds": 300}, err, 200)
	})
}
