package application

import (
	"fmt"
	"sort"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// SchedulingPolicy scores an already-filtered whole-GPU candidate; lower wins.
type SchedulingPolicy interface {
	Score(server domain.Server, matching, selected []domain.GPU) float64
}

type scoreFunc func(domain.Server, []domain.GPU, []domain.GPU) float64

func (f scoreFunc) Score(s domain.Server, m, chosen []domain.GPU) float64 { return f(s, m, chosen) }

// Scheduler plans placement without mutating resources. Repository commit owns reservation.
type Scheduler struct {
	defaultStrategy domain.SchedulingStrategy
	offlineAfter    time.Duration
	policies        map[domain.SchedulingStrategy]SchedulingPolicy
}

// NewScheduler composes deterministic policies shared by the server and benchmark.
func NewScheduler(strategy domain.SchedulingStrategy, offlineAfter time.Duration) Scheduler {
	return Scheduler{defaultStrategy: strategy, offlineAfter: offlineAfter, policies: map[domain.SchedulingStrategy]SchedulingPolicy{
		domain.StrategyFirstFit: scoreFunc(func(_ domain.Server, _, _ []domain.GPU) float64 { return 0 }),
		domain.StrategyBestFit: scoreFunc(func(_ domain.Server, matching, selected []domain.GPU) float64 {
			return float64(len(matching) - len(selected))
		}),
		domain.StrategyBinPack: scoreFunc(func(server domain.Server, _, selected []domain.GPU) float64 {
			free := freeGPUCount(server)
			return float64((free-len(selected))*10 - (len(server.GPUs) - free))
		}),
		domain.StrategyFragmentation: scoreFunc(fragmentationScore),
	}}
}

// WithPolicy returns an independent registry with a replacement or additional scorer.
func (s Scheduler) WithPolicy(name domain.SchedulingStrategy, policy SchedulingPolicy) Scheduler {
	registry := make(map[domain.SchedulingStrategy]SchedulingPolicy, len(s.policies)+1)
	for k, v := range s.policies {
		registry[k] = v
	}
	registry[name] = policy
	s.policies = registry
	return s
}

type candidate struct {
	server domain.Server
	gpus   []domain.GPU
	score  float64
}

// Plan separates filter, score and select; stable UUID/server-ID ties make replay deterministic.
func (s Scheduler) Plan(job domain.Job, servers []domain.Server, now time.Time) (domain.Placement, error) {
	strategy := s.defaultStrategy
	policy, ok := s.policies[strategy]
	if !ok || job.Resources.GPUCount <= 0 {
		return domain.Placement{}, fmt.Errorf("%w: invalid strategy or GPU count", domain.ErrInvalidInput)
	}
	candidates, reason := s.filter(job, servers, now)
	if len(candidates) == 0 {
		return domain.Placement{}, fmt.Errorf("%w: %s", domain.ErrInsufficientGPU, reason)
	}
	for i := range candidates {
		matching := candidates[i].gpus
		selected := matching[:job.Resources.GPUCount]
		candidates[i].score = policy.Score(candidates[i].server, matching, selected)
		candidates[i].gpus = selected
	}
	winner := selectCandidate(candidates)
	uuids := make([]string, len(winner.gpus))
	for i, gpu := range winner.gpus {
		uuids[i] = gpu.UUID
	}
	return domain.Placement{ServerID: winner.server.ID, GPUUUIDs: uuids, Strategy: strategy, Score: winner.score,
		Reason: fmt.Sprintf("%s: server %s, score %.4f; %d whole GPU(s), external/unknown consumers excluded", strategy, winner.server.Name, winner.score, len(uuids))}, nil
}

// filter reports the furthest satisfied constraint so queued jobs have actionable reasons.
func (s Scheduler) filter(job domain.Job, servers []domain.Server, now time.Time) ([]candidate, string) {
	result := make([]candidate, 0)
	online, labels, model, healthy, available, vram := false, false, false, false, false, false
	external, unknown := false, false
	for _, server := range servers {
		if !server.Schedulable(now, s.offlineAfter) {
			continue
		}
		online = true
		if !labelsMatch(server.Labels, job.ServerSelector) {
			continue
		}
		labels = true
		for _, gpu := range server.GPUs {
			if !job.Resources.CapabilityMatches(gpu) {
				continue
			}
			model = true
			if !gpu.Healthy {
				continue
			}
			healthy = true
			external = external || gpu.State == domain.GPUOccupiedLegacy
			unknown = unknown || gpu.State == domain.GPUOccupiedUnknown
			if !gpu.Schedulable() {
				continue
			}
			available = true
			if gpu.AvailableMemoryMiB() >= job.Resources.MinVRAMMiB {
				vram = true
			}
		}
		matching := matchingGPUs(server.GPUs, job.Resources)
		if len(matching) >= job.Resources.GPUCount {
			result = append(result, candidate{server: server, gpus: matching})
		}
	}
	reason := "insufficient GPU count on one server"
	switch {
	case !online:
		reason = "no online agent with fresh inventory (offline, drained or inventory unavailable)"
	case !labels:
		reason = "no server matches the requested labels"
	case !model:
		reason = "GPU model unavailable for performance profile / FP8 requirement"
	case !healthy:
		reason = "no healthy matching GPU"
	case !available && external:
		reason = "GPU occupied by external workload"
	case !available && unknown:
		reason = "GPU ownership unknown; unsafe to schedule"
	case !available:
		reason = "matching GPUs already reserved or allocated"
	case !vram:
		reason = "insufficient available VRAM per GPU"
	case external:
		reason += "; external workloads occupy part of the pool"
	case unknown:
		reason += "; unknown GPU usage is protected"
	}
	return result, reason
}

func selectCandidate(candidates []candidate) candidate {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].server.ID < candidates[j].server.ID
		}
		return candidates[i].score < candidates[j].score
	})
	return candidates[0]
}

func matchingGPUs(gpus []domain.GPU, request domain.ResourceRequest) []domain.GPU {
	result := make([]domain.GPU, 0)
	for _, gpu := range gpus {
		if !request.Matches(gpu) {
			continue
		}
		result = append(result, gpu)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AvailableMemoryMiB() == result[j].AvailableMemoryMiB() {
			return result[i].UUID < result[j].UUID
		}
		return result[i].AvailableMemoryMiB() < result[j].AvailableMemoryMiB()
	})
	return result
}

// fragmentationScore penalizes opening an empty host, then leaving a partial host.
// Bounded terms guarantee those preferences regardless of the GPU count.
func fragmentationScore(server domain.Server, _, selected []domain.GPU) float64 {
	free := freeGPUCount(server)
	remaining := free - len(selected)
	score := 0.0
	if free == len(server.GPUs) {
		score += 2
	}
	if remaining > 0 {
		score += 1
	}
	return score + float64(remaining)/float64(len(server.GPUs)+1)
}

func freeGPUCount(server domain.Server) int {
	free := 0
	for _, gpu := range server.GPUs {
		if gpu.Schedulable() {
			free++
		}
	}
	return free
}

func labelsMatch(actual, requested map[string]string) bool {
	for key, value := range requested {
		if actual[key] != value {
			return false
		}
	}
	return true
}
