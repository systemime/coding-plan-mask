package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRuntimeUsesPrivateFilePermissions(t *testing.T) {
	runtime := NewSessionRuntime(t.TempDir(), 0)
	if _, err := runtime.Append("session-1", "user", "secret", nil, "", nil, nil); err != nil {
		t.Fatalf("append: %v", err)
	}

	path := filepath.Join(runtime.baseDir, "sessions", "full", "session-1.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat full track: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected full track file mode 0600, got %o", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat full track dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("expected full track dir mode 0700, got %o", got)
	}
}

func TestSessionRuntimeLoadsLongMessages(t *testing.T) {
	runtime := NewSessionRuntime(t.TempDir(), 0)
	longText := strings.Repeat("x", 128*1024)
	if _, err := runtime.Append("session-long", "user", longText, nil, "", nil, nil); err != nil {
		t.Fatalf("append: %v", err)
	}

	records, err := runtime.LoadTrack("session-long", "full", 0)
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	if len(records) != 1 || records[0].Content != longText {
		t.Fatalf("unexpected records: len=%d", len(records))
	}
}

func TestSafeSessionIDFallsBackWhenSanitizedEmpty(t *testing.T) {
	if got := safeSessionID("////"); got != "____" {
		t.Fatalf("expected slash characters to be sanitized, got %q", got)
	}
	if got := safeSessionID(strings.Repeat("a", 200)); len(got) != maxSessionIDBytes {
		t.Fatalf("expected long session id to be capped at %d bytes, got %d", maxSessionIDBytes, len(got))
	}
}
