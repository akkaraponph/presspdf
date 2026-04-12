package folio

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExternalTool represents a discovered command-line tool on the system PATH.
type ExternalTool struct {
	Name string // binary name (e.g. "pdftoppm", "tesseract")
	Path string // absolute path from exec.LookPath
}

// Run executes the tool with the given arguments and returns combined
// stdout+stderr output. Returns a *ToolError on non-zero exit.
func (t *ExternalTool) Run(args ...string) ([]byte, error) {
	cmd := exec.Command(t.Path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, &ToolError{Tool: t.Name, Args: args, Err: err, Output: out}
	}
	return out, nil
}

// ToolError wraps an external tool execution failure with context.
type ToolError struct {
	Tool   string
	Args   []string
	Err    error
	Output []byte
}

func (e *ToolError) Error() string {
	msg := fmt.Sprintf("folio: %s failed: %v", e.Tool, e.Err)
	if len(e.Output) > 0 {
		msg += ": " + strings.TrimSpace(string(e.Output))
	}
	return msg
}

func (e *ToolError) Unwrap() error { return e.Err }

// FindTool searches the system PATH for the first available tool from
// the given binary names. Returns the first match or a *ToolNotFoundError
// listing all candidates tried.
func FindTool(names ...string) (*ExternalTool, error) {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return &ExternalTool{Name: name, Path: p}, nil
		}
	}
	return nil, &ToolNotFoundError{Tried: names}
}

// ToolNotFoundError is returned when none of the requested tools are
// available on PATH.
type ToolNotFoundError struct {
	Tried []string
}

func (e *ToolNotFoundError) Error() string {
	return fmt.Sprintf("folio: no tool found on PATH (tried: %s)", strings.Join(e.Tried, ", "))
}

// TempDir creates a temporary directory with the given pattern and returns
// its path along with a cleanup function that removes the directory tree.
// The caller should defer cleanup().
func TempDir(pattern string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", pattern)
	if err != nil {
		return "", func() {}, fmt.Errorf("folio: create temp dir: %w", err)
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// CollectFiles gathers files matching the given extension from a directory,
// returned in sorted order (os.ReadDir sorts by name).
func CollectFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("folio: read output dir: %w", err)
	}

	ext = strings.ToLower(ext)
	var paths []string
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ext) {
			paths = append(paths, fmt.Sprintf("%s/%s", dir, e.Name()))
		}
	}
	return paths, nil
}
