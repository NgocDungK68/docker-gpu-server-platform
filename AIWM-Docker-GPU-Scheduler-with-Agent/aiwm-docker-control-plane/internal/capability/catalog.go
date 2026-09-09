// Package capability resolves user profiles to immutable physical constraints.
package capability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type Profile struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Models []Model `json:"-"`
}
type Model struct {
	Name string
	FP8  bool
}
type Resolver interface {
	Profiles() []Profile
	Resolve(profile string, fp8 bool) ([]string, error)
}
type Catalog struct{ profiles []Profile }

// Default contains centrally curated compatibility groups, not measured performance equivalence.
func Default() *Catalog {
	a100 := []Model{{"A100", false}, {"NVIDIA-A100-80GB", false}, {"NVIDIA A100-SXM4-40GB", false}, {"NVIDIA A100-SXM4-80GB", false}, {"NVIDIA A100 80GB PCIe", false}}
	t4 := []Model{{"T4", false}, {"Tesla T4", false}, {"NVIDIA T4", false}}
	h100 := []Model{{"H100", true}, {"NVIDIA H100 80GB HBM3", true}, {"NVIDIA H100 PCIe", true}}
	return New([]Profile{{"general", "GPU tổng quát", append(append(t4, a100...), h100...)}, {"a100-equivalent", "A100-equivalent", a100}, {"h100-equivalent", "H100-equivalent", h100}})
}
func New(profiles []Profile) *Catalog {
	copyProfiles := append([]Profile(nil), profiles...)
	for i := range copyProfiles {
		copyProfiles[i].Models = append([]Model(nil), profiles[i].Models...)
	}
	return &Catalog{profiles: copyProfiles}
}
func (c *Catalog) Profiles() []Profile {
	result := make([]Profile, 0, len(c.profiles))
	for _, p := range c.profiles {
		result = append(result, Profile{ID: p.ID, Label: p.Label})
	}
	return result
}
func (c *Catalog) Resolve(profile string, fp8 bool) ([]string, error) {
	for _, p := range c.profiles {
		if p.ID != profile {
			continue
		}
		models := make([]string, 0)
		for _, m := range p.Models {
			if !fp8 || m.FP8 {
				models = append(models, strings.TrimSpace(m.Name))
			}
		}
		sort.Strings(models)
		return models, nil // an existing profile may have no FP8-capable model
	}
	return nil, fmt.Errorf("%w: unknown performance profile", domain.ErrInvalidInput)
}
