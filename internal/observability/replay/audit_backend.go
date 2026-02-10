package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

var (
	ErrReplayAuditLogRootRequired = errors.New("replay audit log root is required")
	ErrReplayAuditTenantRequired  = errors.New("replay audit tenant_id is required")
)

const (
	defaultReplayAuditDirMode  os.FileMode = 0o755
	defaultReplayAuditFileMode os.FileMode = 0o600
)

// JSONLFileAuditBackend appends immutable replay-audit events as JSON lines.
type JSONLFileAuditBackend struct {
	Path     string
	DirMode  os.FileMode
	FileMode os.FileMode
	Marshal  func(event obs.ReplayAuditEvent) ([]byte, error)

	mu sync.Mutex
}

// AppendReplayAuditEvent appends one validated replay-audit event to disk.
func (b *JSONLFileAuditBackend) AppendReplayAuditEvent(event obs.ReplayAuditEvent) error {
	if b == nil {
		return fmt.Errorf("jsonl replay audit backend is required")
	}
	if b.Path == "" {
		return fmt.Errorf("replay audit backend path is required")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid replay audit event: %w", err)
	}

	marshal := b.Marshal
	if marshal == nil {
		marshal = func(event obs.ReplayAuditEvent) ([]byte, error) {
			return json.Marshal(event)
		}
	}

	payload, err := marshal(event)
	if err != nil {
		return fmt.Errorf("marshal replay audit event: %w", err)
	}

	dirMode := b.DirMode
	if dirMode == 0 {
		dirMode = defaultReplayAuditDirMode
	}
	fileMode := b.FileMode
	if fileMode == 0 {
		fileMode = defaultReplayAuditFileMode
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(b.Path), dirMode); err != nil {
		return fmt.Errorf("create replay audit directory: %w", err)
	}
	f, err := os.OpenFile(b.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open replay audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("append replay audit event: %w", err)
	}
	return nil
}

// JSONLFileAuditBackendResolver resolves tenant-scoped JSONL replay-audit backends.
type JSONLFileAuditBackendResolver struct {
	RootDir        string
	DirMode        os.FileMode
	FileMode       os.FileMode
	TenantFileName func(tenantID string) (string, error)
}

// ResolveReplayAuditBackend resolves a tenant-specific JSONL replay-audit backend.
func (r JSONLFileAuditBackendResolver) ResolveReplayAuditBackend(tenantID string) (ImmutableReplayAuditSink, error) {
	path, err := r.tenantPath(tenantID)
	if err != nil {
		return nil, err
	}
	return &JSONLFileAuditBackend{
		Path:     path,
		DirMode:  r.effectiveDirMode(),
		FileMode: r.effectiveFileMode(),
	}, nil
}

// TenantLogPath returns the resolved tenant log path for diagnostics/tests.
func (r JSONLFileAuditBackendResolver) TenantLogPath(tenantID string) (string, error) {
	return r.tenantPath(tenantID)
}

func (r JSONLFileAuditBackendResolver) tenantPath(tenantID string) (string, error) {
	rootDir := strings.TrimSpace(r.RootDir)
	if rootDir == "" {
		return "", ErrReplayAuditLogRootRequired
	}
	filenameFn := r.TenantFileName
	if filenameFn == nil {
		filenameFn = defaultReplayAuditTenantFilename
	}
	filename, err := filenameFn(tenantID)
	if err != nil {
		return "", err
	}
	if filename == "" {
		return "", fmt.Errorf("replay audit filename is required")
	}
	cleanRoot := filepath.Clean(rootDir)
	cleanPath := filepath.Join(cleanRoot, filename)
	return cleanPath, nil
}

func (r JSONLFileAuditBackendResolver) effectiveDirMode() os.FileMode {
	if r.DirMode == 0 {
		return defaultReplayAuditDirMode
	}
	return r.DirMode
}

func (r JSONLFileAuditBackendResolver) effectiveFileMode() os.FileMode {
	if r.FileMode == 0 {
		return defaultReplayAuditFileMode
	}
	return r.FileMode
}

func defaultReplayAuditTenantFilename(tenantID string) (string, error) {
	normalized := strings.TrimSpace(tenantID)
	if normalized == "" {
		return "", ErrReplayAuditTenantRequired
	}

	var b strings.Builder
	for _, r := range normalized {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "", fmt.Errorf("tenant_id must include at least one alphanumeric character")
	}
	return name + ".replay-audit.jsonl", nil
}
