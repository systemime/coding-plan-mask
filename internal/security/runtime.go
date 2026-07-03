package security

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	maxAuditLineBytes = 16 * 1024 * 1024
	auditTrimMinBytes = 8 * 1024 * 1024
)

type SessionEntry struct {
	Role        string            `json:"role"`
	FullText    string            `json:"full_text,omitempty"`
	CleanText   string            `json:"clean_text,omitempty"`
	TimestampMS *int64            `json:"timestamp_ms,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type TrackRecord struct {
	Role        string            `json:"role"`
	Content     string            `json:"content"`
	TimestampMS *int64            `json:"timestamp_ms,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type SessionRuntime struct {
	baseDir    string
	maxEntries int
	lockMu     sync.Mutex
	locks      map[string]*sync.Mutex
}

func NewSessionRuntime(baseDir string, maxEntries int) *SessionRuntime {
	if maxEntries < 0 {
		maxEntries = 0
	}
	return &SessionRuntime{
		baseDir:    baseDir,
		maxEntries: maxEntries,
		locks:      map[string]*sync.Mutex{},
	}
}

func (r *SessionRuntime) Append(sessionID, role, fullText string, cleanText *string, placeholder string, timestampMS *int64, metadata map[string]string) (SessionEntry, error) {
	if cleanText != nil && placeholder != "" {
		return SessionEntry{}, fmt.Errorf("clean_text and placeholder are mutually exclusive")
	}
	clean := fullText
	if cleanText != nil {
		clean = *cleanText
	} else if placeholder != "" {
		clean = placeholder
	}
	entry := SessionEntry{
		Role:        role,
		FullText:    fullText,
		CleanText:   clean,
		TimestampMS: timestampMS,
		Metadata:    cloneMetadata(metadata),
	}
	if err := r.appendTrack(sessionID, "full", TrackRecord{Role: role, Content: fullText, TimestampMS: timestampMS, Metadata: cloneMetadata(metadata)}); err != nil {
		return SessionEntry{}, err
	}
	if err := r.appendTrack(sessionID, "clean", TrackRecord{Role: role, Content: clean, TimestampMS: timestampMS, Metadata: cloneMetadata(metadata)}); err != nil {
		return SessionEntry{}, err
	}
	return entry, nil
}

func (r *SessionRuntime) LoadTrack(sessionID, track string, limit int) ([]TrackRecord, error) {
	if track != "full" && track != "clean" {
		return nil, fmt.Errorf("track must be 'full' or 'clean'")
	}
	path := r.trackPath(sessionID, track)
	lock := r.getLock(path)
	lock.Lock()
	defer lock.Unlock()

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []TrackRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxAuditLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item TrackRecord
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			records = append(records, item)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	return records, nil
}

func (r *SessionRuntime) ListSessions() ([]string, error) {
	seen := map[string]struct{}{}
	for _, track := range []string{"full", "clean"} {
		root := filepath.Join(r.baseDir, "sessions", track)
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if name != "" {
				seen[name] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (r *SessionRuntime) appendTrack(sessionID, track string, record TrackRecord) error {
	if track != "full" && track != "clean" {
		return fmt.Errorf("track must be 'full' or 'clean'")
	}
	path := r.trackPath(sessionID, track)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Dir(path), 0700)
	lock := r.getLock(path)
	lock.Lock()
	defer lock.Unlock()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_ = os.Chmod(path, 0600)

	line, err := json.Marshal(record)
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return err
	}

	if r.maxEntries > 0 {
		info, statErr := file.Stat()
		if closeErr := file.Close(); closeErr != nil {
			return closeErr
		}
		if statErr == nil && info.Size() > auditTrimMinBytes {
			if err := trimTrackFile(path, r.maxEntries); err != nil {
				return err
			}
		}
	} else {
		if err := file.Close(); err != nil {
			return err
		}
	}

	return nil
}

func trimTrackFile(path string, maxEntries int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) <= maxEntries {
		return nil
	}
	kept := strings.Join(lines[len(lines)-maxEntries:], "\n") + "\n"
	return os.WriteFile(path, []byte(kept), 0600)
}

func (r *SessionRuntime) trackPath(sessionID, track string) string {
	return filepath.Join(r.baseDir, "sessions", track, safeSessionID(sessionID)+".jsonl")
}

func (r *SessionRuntime) getLock(path string) *sync.Mutex {
	r.lockMu.Lock()
	defer r.lockMu.Unlock()
	lock := r.locks[path]
	if lock == nil {
		lock = &sync.Mutex{}
		r.locks[path] = lock
	}
	return lock
}

func safeSessionID(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range sessionID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(metadata))
	for k, v := range metadata {
		out[k] = v
	}
	return out
}
