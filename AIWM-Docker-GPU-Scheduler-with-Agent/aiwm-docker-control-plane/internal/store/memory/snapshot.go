package memory

import "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"

// Snapshot is the versioned private persistence representation, never an HTTP DTO.
type Snapshot struct {
	Version  int
	Servers  map[string]domain.Server
	Machines map[string]string
	Jobs     map[string]domain.Job
	Commands map[string]domain.Command
}

// Export takes a detached consistent snapshot for durable transactions.
func (s *Store) Export() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Snapshot{Version: 1, Servers: map[string]domain.Server{}, Machines: map[string]string{}, Jobs: map[string]domain.Job{}, Commands: map[string]domain.Command{}}
	for k, v := range s.servers {
		out.Servers[k] = cloneServer(v)
	}
	for k, v := range s.machines {
		out.Machines[k] = v
	}
	for k, v := range s.jobs {
		out.Jobs[k] = cloneJob(v)
	}
	for k, v := range s.commands {
		out.Commands[k] = cloneCommand(v)
	}
	return out
}

// Restore replaces private state under the lock. Durable adapter validates the format first.
func (s *Store) Restore(snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers, s.machines, s.jobs, s.commands = snapshot.Servers, snapshot.Machines, snapshot.Jobs, snapshot.Commands
}
