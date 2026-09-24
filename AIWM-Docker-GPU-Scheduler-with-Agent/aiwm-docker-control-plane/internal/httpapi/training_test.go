package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/objectstore/s3"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/durable"
)

type httpTrainingObjects struct{ data []byte }

func (s *httpTrainingObjects) Put(_ context.Context, key string, r io.Reader, _ int64) (string, error) {
	data, err := io.ReadAll(r)
	s.data = data
	return "s3://test/" + key, err
}
func (s *httpTrainingObjects) DownloadURL(context.Context, string, time.Duration) (string, error) {
	return "https://signed.invalid/model", nil
}

type trainingHTTPFixture struct {
	server   *httptest.Server
	cp       *application.ControlPlane
	repo     *durable.Store
	ctx      context.Context
	sequence uint64
}

func newTrainingHTTP(t *testing.T, objects ports.ObjectStore) *trainingHTTPFixture {
	t.Helper()
	repo, err := durable.Open(filepath.Join(t.TempDir(), "runtime.gob"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	m := &identityMetadata{sessions: map[string]domain.Principal{}}
	for token, org := range map[string]string{"own-session": "own", "other-session": "other"} {
		sum := sha256.Sum256([]byte(token))
		m.sessions[hex.EncodeToString(sum[:])] = domain.Principal{User: domain.User{ID: token, OrganizationID: org, Role: domain.RoleOrganizationUser, Enabled: true}, Organization: domain.Organization{ID: org, Enabled: true}}
	}
	httpServer := httptest.NewUnstartedServer(nil)
	opts := application.DefaultTrainingOptions()
	opts.APIURL = "http://" + httpServer.Listener.Addr().String()
	cp := application.New(repo, application.Options{ObjectStore: objects, Training: opts, OfflineAfter: time.Minute, CommandLease: time.Second})
	httpServer.Config.Handler = New(cp, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, Options{Identity: application.NewIdentity(m)}).Handler()
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	f := &trainingHTTPFixture{server: httpServer, cp: cp, repo: repo, ctx: domain.WithPrincipal(context.Background(), domain.Principal{User: domain.User{OrganizationID: "own", Role: domain.RoleOrganizationUser}})}
	f.addServer(t, "s1", "GPU-1")
	return f
}
func (f *trainingHTTPFixture) addServer(t *testing.T, id, gpu string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := f.repo.UpsertServer(f.ctx, domain.Server{ID: id, MachineID: "m-" + id, OrganizationID: "own", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{{UUID: gpu, Model: "A100", State: domain.GPUFree, Healthy: true, MemoryTotalMiB: 40960}}})
	if err != nil {
		t.Fatal(err)
	}
}
func (f *trainingHTTPFixture) start(t *testing.T, j domain.Job) (domain.Job, agentv1.StartContainerPayload) {
	t.Helper()
	if _, err := f.cp.ScheduleOnce(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, err := f.repo.GetJob(f.ctx, j.ID)
	if err != nil || j.Assignment == nil {
		t.Fatalf("no reservation: %v", err)
	}
	cmds, err := f.repo.LeaseCommands(f.ctx, j.Assignment.ServerID, time.Now().UTC(), time.Second, 10)
	if err != nil || len(cmds) != 1 || cmds[0].Type != domain.CommandStartContainer {
		t.Fatalf("no START: %v", err)
	}
	var payload agentv1.StartContainerPayload
	if err = json.Unmarshal(cmds[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.AckCommand(f.ctx, j.Assignment.ServerID, cmds[0].ID, domain.CommandAckRequest{Succeeded: true, ContainerID: "container-" + j.ID}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.observe(t, j, "running")
	j, _ = f.repo.GetJob(f.ctx, j.ID)
	return j, payload
}
func (f *trainingHTTPFixture) observe(t *testing.T, j domain.Job, state string) {
	t.Helper()
	f.sequence++
	code := 0
	server, err := f.repo.GetServer(f.ctx, j.Assignment.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	container := domain.Container{ID: "container-" + j.ID, JobID: j.ID, Origin: domain.ContainerManaged, State: state, GPUUUIDs: j.Assignment.GPUUUIDs}
	if state == "exited" {
		container.ExitCode = &code
	}
	_, err = f.repo.ReplaceInventory(f.ctx, server.ID, domain.InventoryReport{Sequence: f.sequence, ObservedAt: time.Now().UTC(), ReceivedAt: time.Now().UTC(), GPUs: server.GPUs, Containers: []domain.Container{container}})
	if err != nil {
		t.Fatal(err)
	}
}
func TestTrainingHTTPAuthorizationAndPublication(t *testing.T) {
	objects := &httpTrainingObjects{}
	f := newTrainingHTTP(t, objects)
	input := validAllocationRequest()
	j, err := f.cp.CreateJob(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	j, _ = f.start(t, j)
	handler := f.server.Config.Handler
	for _, token := range []string{"", "own-session", "other-session", "wrong-token"} {
		w := identityRequest(handler, "PUT", "/api/v1/training/"+j.ID+"/artifact", token, []byte("bad"))
		if w.Code != 401 {
			t.Fatalf("training auth: %d", w.Code)
		}
	}
	data := bytes.Repeat([]byte("a"), 2<<20) // Must pass the normal JSON 1 MiB limit safely.
	w := identityRequest(handler, "PUT", "/api/v1/training/"+j.ID+"/artifact", j.TrainingToken, data)
	if w.Code != 200 || !bytes.Equal(data, objects.data) || !strings.Contains(w.Body.String(), "\"READY\"") {
		t.Fatalf("upload %d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 401}, {"other-session", 404}, {"own-session", 200}} {
		w = identityRequest(handler, "GET", "/api/v1/jobs/"+j.ID+"/artifact", tc.token, nil)
		if w.Code != tc.code {
			t.Fatalf("download authorization %s: %d %s", tc.token, w.Code, w.Body)
		}
	}
	w = identityRequest(handler, "GET", "/api/v1/jobs/"+j.ID, "own-session", nil)
	if strings.Contains(w.Body.String(), j.TrainingToken) || strings.Contains(w.Body.String(), "uploadId") {
		t.Fatal("private token/lease exposed")
	}
	w = identityRequest(handler, "POST", "/api/v1/jobs/"+j.ID+"/continue", "other-session", []byte("{}"))
	if w.Code != 404 {
		t.Fatal("cross-org continuation allowed")
	}
}

// Opt-in: real MinIO + real Python training app + HTTP + durable metadata.
// GPU/Agent observations are controlled at the repository boundary, not claimed as GPU E2E.
func TestTrainingMinIOContract(t *testing.T) {
	endpoint := os.Getenv("AIWM_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set AIWM_TEST_S3_ENDPOINT for MinIO contract integration")
	}
	python := os.Getenv("AIWM_TEST_PYTHON")
	if python == "" {
		python = "python"
	}
	if _, err := exec.LookPath(python); err != nil {
		t.Fatal("Python required for training contract integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	objects, err := s3.New(ctx, s3.Config{Endpoint: endpoint, AccessKey: os.Getenv("AIWM_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("AIWM_TEST_S3_SECRET_KEY"), Bucket: fmt.Sprintf("aiwm-test-%d", time.Now().UnixNano()), CreateBucket: true})
	if err != nil {
		t.Fatal(err)
	}
	f := newTrainingHTTP(t, objects)
	input := validAllocationRequest()
	input.NeededAt = time.Now().UTC().Format(time.RFC3339Nano)
	input.TTLSeconds = 3
	input.Image = "aiwm-training-demo:local"
	input.Command = []string{"--steps", "40", "--step-seconds", "0.1", "--checkpoint-every", "2"}
	job, err := f.cp.CreateJob(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	job, payload := f.start(t, job)
	script, err := filepath.Abs("../../../../demo/training/train.py")
	if err != nil {
		t.Fatal(err)
	}
	run := func(job domain.Job, payload agentv1.StartContainerPayload, expectExpiry bool) {
		t.Helper()
		command := exec.CommandContext(ctx, python, append([]string{"-u", script}, payload.Command...)...)
		command.Env = os.Environ()
		for k, v := range payload.Environment {
			command.Env = append(command.Env, k+"="+v)
		}
		var output bytes.Buffer
		command.Stdout = &output
		command.Stderr = &output
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("training app: %v; %s", err, output.String())
				}
				latest, _ := f.repo.GetJob(f.ctx, job.ID)
				if expectExpiry && (latest.Status != domain.JobStopping || latest.TerminationReason != domain.TerminationTimeLimit) {
					t.Fatal("app exited before TIME_LIMIT stop")
				}
				f.observe(t, job, "exited")
				return
			case <-tick.C:
				if _, err := f.cp.ScheduleOnce(f.ctx); err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("training app timeout")
			}
		}
	}
	run(job, payload, true)
	source, _ := f.repo.GetJob(f.ctx, job.ID)
	if !source.Resumable() || source.Training.CheckpointStep < 1 {
		t.Fatal("no resumable checkpoint")
	}
	if _, err = f.repo.SetServerDrained(f.ctx, "s1", true); err != nil {
		t.Fatal(err)
	}
	f.addServer(t, "s2", "GPU-2")
	body, _ := json.Marshal(application.ContinueTrainingRequest{NeededAt: time.Now().UTC().Format(time.RFC3339Nano), TTLSeconds: 30})
	w := identityRequest(f.server.Config.Handler, "POST", "/api/v1/jobs/"+source.ID+"/continue", "own-session", body)
	if w.Code != 201 {
		t.Fatalf("continuation: %d %s", w.Code, w.Body)
	}
	var response struct{ Data JobView }
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	next, _ := f.repo.GetJob(f.ctx, response.Data.ID)
	next, payload = f.start(t, next)
	if next.Assignment.ServerID != "s2" || payload.Environment["AIWM_RESUME_CHECKPOINT_URI"] != source.Training.LatestCheckpointURI {
		t.Fatal("resume lost across placement")
	}
	run(next, payload, false)
	final, _ := f.repo.GetJob(f.ctx, next.ID)
	if final.Status != domain.JobSucceeded || final.TerminationReason != domain.TerminationCompleted || final.Training.ArtifactStatus != domain.ArtifactReady {
		t.Fatal("final artifact not ready")
	}
	w = identityRequest(f.server.Config.Handler, "GET", "/api/v1/jobs/"+final.ID+"/artifact", "own-session", nil)
	var download struct{ Data struct{ URL string } }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &download) != nil {
		t.Fatal("no signed download")
	}
	res, err := (&http.Client{Timeout: 5 * time.Second}).Get(download.Data.URL)
	if err != nil {
		t.Fatal("cannot download signed artifact")
	}
	defer res.Body.Close()
	var model struct {
		TrainedSteps int
		DemoModel    bool
	}
	if err = json.NewDecoder(res.Body).Decode(&model); err != nil || res.StatusCode != 200 || !model.DemoModel || model.TrainedSteps != 40 || int(source.Training.CheckpointStep) >= model.TrainedSteps {
		t.Fatal("download/resume data incorrect")
	}
	server, _ := f.repo.GetServer(f.ctx, "s2")
	if server.GPUs[0].State != domain.GPUFree {
		t.Fatal("GPU not released")
	}
	t.Log("PASS: warning -> real MinIO checkpoint -> TIME_LIMIT -> new server resume -> final artifact -> signed download -> GPU release")
}
