package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFiles_MissingFilesAreNonFatal(t *testing.T) {
	loadDotEnvFiles("/tmp/definitely-missing-dotenv-file")
}

func TestLoadDotEnvFiles_LoadsExistingFile(t *testing.T) {
	const key = "LOAD_DOTENV_TEST_KEY_UNLIKELY_20260428"
	_ = os.Unsetenv(key)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(key+"=loaded\n"), 0o600); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	loadDotEnvFiles(path)
	if got := os.Getenv(key); got != "loaded" {
		t.Fatalf("expected key loaded from env file, got %q", got)
	}
}
