package domain

import (
	"slices"
	"time"
)

// TimeWindow uses half-open intervals: touching endpoints do not conflict.
type TimeWindow struct {
	StartAt time.Time `json:"requestedStartAt"`
	EndAt   time.Time `json:"requestedEndAt"`
}

func (w TimeWindow) Valid() bool { return !w.StartAt.IsZero() && w.StartAt.Before(w.EndAt) }
func (w TimeWindow) Contains(at time.Time) bool {
	return w.Valid() && !at.Before(w.StartAt) && at.Before(w.EndAt)
}
func (w TimeWindow) Overlaps(other TimeWindow) bool {
	return w.StartAt.Before(other.EndAt) && other.StartAt.Before(w.EndAt)
}

// RequestedWindow reuses ttlSeconds as the GPU allocation duration, not container uptime.
func (j Job) RequestedWindow() TimeWindow {
	start, err := time.Parse(time.RFC3339, j.NeededAt)
	if err != nil || j.TTLSeconds <= 0 || j.TTLSeconds > int64((1<<63-1)/time.Second) {
		return TimeWindow{}
	}
	return TimeWindow{StartAt: start.UTC(), EndAt: start.Add(time.Duration(j.TTLSeconds) * time.Second).UTC()}
}
func (a Assignment) Window() TimeWindow { return TimeWindow{a.StartAt, a.EndAt} }

// Replannable never includes execution in flight or reservations whose start has arrived.
func (j Job) Replannable(now time.Time) bool {
	return j.Status == JobQueued || (j.Status == JobAssigned && j.Assignment != nil &&
		j.Assignment.ReservationState == "PLANNED" && j.Assignment.CommandID == "" && j.Assignment.StartAt.After(now))
}

type ReservationPlan struct {
	JobID     string
	Placement *Placement
	Policy    PolicyDecision
	Reason    string
}

// CalendarServers is a planning projection only. It never changes physical inventory.
// Managed allocations with a known end may be planned after that end; execution must
// independently require actual free GPUs. External/unknown occupancy remains blocking.
func CalendarServers(job Job, servers []Server, bookings []Job, now time.Time, offlineAfter time.Duration) []Server {
	window := job.RequestedWindow()
	result := make([]Server, 0, len(servers))
	if !window.Valid() {
		return result
	}
	for _, server := range servers {
		if !SameOrganization(job.OrganizationID, server.OrganizationID) {
			continue
		}
		if !server.Schedulable(now, offlineAfter) {
			continue
		}
		server.GPUs = append([]GPU(nil), server.GPUs...)
		for i := range server.GPUs {
			gpu := &server.GPUs[i]
			if !gpu.Healthy || !job.Resources.CapabilityMatches(*gpu) || gpu.MemoryTotalMiB < job.Resources.MinVRAMMiB {
				continue
			}
			if window.StartAt.After(now) && (gpu.State == GPUAllocated || gpu.State == GPUReserved) {
				for _, owner := range bookings {
					a := owner.Assignment
					if owner.ID == gpu.AssignedJobID && !owner.Status.Terminal() && a != nil && a.ServerID == server.ID &&
						a.Window().Valid() && !a.EndAt.After(window.StartAt) && slices.Contains(a.GPUUUIDs, gpu.UUID) {
						gpu.State, gpu.AssignedJobID, gpu.MemoryUsedMiB = GPUFree, "", 0
						break
					}
				}
			}
			for _, booked := range bookings {
				a := booked.Assignment
				if booked.ID == job.ID || booked.Status.Terminal() || a == nil || a.ReleasedAt != nil || a.ServerID != server.ID {
					continue
				}
				if slices.Contains(a.GPUUUIDs, gpu.UUID) && (!a.Window().Valid() || window.Overlaps(a.Window())) && gpu.Schedulable() {
					gpu.State, gpu.AssignedJobID = GPUReserved, booked.ID
				}
			}
		}
		result = append(result, server)
	}
	return result
}
