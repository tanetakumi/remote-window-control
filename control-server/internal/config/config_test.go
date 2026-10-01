package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationIsDataAndEnvironmentOverridesFile(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "control-server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "control-server", "go.mod"), []byte("module test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, ".env"), []byte("# test\nSHARE_APP_CLIENT_DIR='literal-$(command)'\nSHARE_APP_ADDR=127.0.0.1:9000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("SHARE_APP_ADDR", "")
	t.Setenv("SHARE_APP_CLIENT_DIR", "web")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" || cfg.ClientDir != filepath.Join(base, "web") {
		t.Fatal(cfg)
	}
	values, err := readConfig(filepath.Join(base, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if values["SHARE_APP_CLIENT_DIR"] != "literal-$(command)" {
		t.Fatal("configuration unexpectedly evaluated")
	}
	// Moving the source directories must still discover the repository and the
	// default UI assets without an explicit directory override.
	if err := os.WriteFile(filepath.Join(base, ".env"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHARE_APP_CLIENT_DIR", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDir != base || cfg.ClientDir != filepath.Join(base, "web-ui", "dist") || cfg.ListenAddr != "127.0.0.1:8443" {
		t.Fatalf("development defaults after relocation: %+v", cfg)
	}
}
