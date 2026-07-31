package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	_ = loadEnvFile(".dev.env")
	if err := setupTestSigning(); err != nil {
		panic("setupTestSigning: " + err.Error())
	}
	code := m.Run()
	os.Exit(code)
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_LOAD_ENV_KEY=hello\n# comment\n\nTEST_LOAD_ENV_KEY2=world\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_LOAD_ENV_KEY", "")
	t.Setenv("TEST_LOAD_ENV_KEY2", "")

	if err := loadEnvFile(envFile); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("TEST_LOAD_ENV_KEY"); got != "hello" {
		t.Errorf("TEST_LOAD_ENV_KEY = %q, want %q", got, "hello")
	}
	if got := os.Getenv("TEST_LOAD_ENV_KEY2"); got != "world" {
		t.Errorf("TEST_LOAD_ENV_KEY2 = %q, want %q", got, "world")
	}
}

func TestLoadEnvFileMissingFileIsOK(t *testing.T) {
	if err := loadEnvFile("/nonexistent/.env"); err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}
