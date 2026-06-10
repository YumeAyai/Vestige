package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsClientConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
client:
  addr: ":18080"
  db_path: "tmp/app.db"
  tracking_base_url: "https://track.example.com/jianji"
  tracking_source_token: "source-1"
  qr_code_target_url: "https://example.com/survey"
scf:
  tcb:
    env_id: "env-1"
    region: "ap-shanghai"
    events_collection: "events"
    assets_collection: "assets"
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Client.Addr != ":18080" {
		t.Fatalf("unexpected client addr: %s", cfg.Client.Addr)
	}
	if cfg.Client.TrackingBaseURL != "https://track.example.com/jianji" {
		t.Fatalf("unexpected tracking base URL: %s", cfg.Client.TrackingBaseURL)
	}
	if cfg.SCF.TCB.EnvID != "env-1" || cfg.SCF.TCB.EventsCollection != "events" {
		t.Fatalf("unexpected tcb config: %#v", cfg.SCF.TCB)
	}
}

func TestDefaultTrackingBaseURLIsRemote(t *testing.T) {
	if got := Default().Client.TrackingBaseURL; got == "" || got == "http://localhost:8081" {
		t.Fatalf("default tracking base URL should not be localhost: %q", got)
	}
}
