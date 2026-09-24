package config

import "testing"

func TestControlPlaneURLPrecedence(t *testing.T) {
	previous := ReleaseControlPlaneURL
	t.Cleanup(func() { ReleaseControlPlaneURL = previous })
	t.Setenv("AIWM_CONTROL_PLANE_URL", "")
	ReleaseControlPlaneURL = ""
	cfg, err := LoadForCheck()
	if err != nil || cfg.ControlPlaneURL != "http://localhost:8080" {
		t.Fatalf("development fallback: %q, %v", cfg.ControlPlaneURL, err)
	}
	ReleaseControlPlaneURL = "https://release.example.test"
	cfg, err = LoadForCheck()
	if err != nil || cfg.ControlPlaneURL != ReleaseControlPlaneURL {
		t.Fatalf("release default: %q, %v", cfg.ControlPlaneURL, err)
	}
	t.Setenv("AIWM_CONTROL_PLANE_URL", "https://override.example.test")
	cfg, err = LoadForCheck()
	if err != nil || cfg.ControlPlaneURL != "https://override.example.test" {
		t.Fatalf("runtime override: %q, %v", cfg.ControlPlaneURL, err)
	}
	t.Setenv("AIWM_CONTROL_PLANE_URL", "invalid")
	if _, err = LoadForCheck(); err == nil {
		t.Fatal("invalid runtime override must fail instead of falling back")
	}
}
