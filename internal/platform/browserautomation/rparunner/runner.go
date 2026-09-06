package rparunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ErrRunnerInfrastructure reports that the subprocess itself failed (could not
// start, produced no parseable result, or was killed). Callers decide whether
// that maps to a failed run or result_unknown based on click progress.
var ErrRunnerInfrastructure = errors.New("rpa runner infrastructure failure")

const stderrCap = 8 << 10

type Runner struct {
	// Command launches the TypeScript runner script, typically
	// []string{"npx", "tsx"} or a repo-local tsx binary path.
	Command []string
	// ScriptPath is the runner script, relative to WorkDir.
	ScriptPath string
	// WorkDir is the process working directory, normally the repository root.
	WorkDir string
	// CDPEndpoint is the DevTools endpoint of the externally authenticated
	// browser session.
	CDPEndpoint string
	// EdgeSessionFile is the persistent local Edge session metadata file.
	// Runner v3 resolves its current WebSocket endpoint immediately before use.
	EdgeSessionFile string

	PrepareTimeout time.Duration
	SubmitTimeout  time.Duration
}

// WithCDPEndpoint returns a copy of the runner bound to a specific DevTools
// endpoint, so one configured runner can serve many environments.
func (r Runner) WithCDPEndpoint(endpoint string) Runner {
	r.CDPEndpoint = endpoint
	return r
}

func (r Runner) WithEdgeSessionFile(path string) Runner {
	r.EdgeSessionFile = path
	return r
}

// Run executes one plan in a subprocess and returns the parsed result. The
// plan travels over stdin; only the result document comes back over stdout.
func (r Runner) Run(ctx context.Context, plan RpaPlan) (RpaResult, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return RpaResult{}, fmt.Errorf("%w: encode plan: %v", ErrRunnerInfrastructure, err)
	}
	return r.runPayload(ctx, payload, plan.Mode, nil)
}

// RunV3 passes a schema-validated v3 plan to the TypeScript runner without
// projecting it through the frozen v2 Go model. The confirmation token is a
// process argument and is never added to the plan or result document.
func (r Runner) RunV3(ctx context.Context, plan json.RawMessage, confirmToken, authorityStateDirectory string) (RpaResult, error) {
	var header struct {
		SchemaVersion string `json:"schema_version"`
		Mode          string `json:"mode"`
	}
	if err := json.Unmarshal(plan, &header); err != nil {
		return RpaResult{}, fmt.Errorf("%w: decode v3 plan header: %v", ErrRunnerInfrastructure, err)
	}
	if header.SchemaVersion != PlanSchemaV3 {
		return RpaResult{}, fmt.Errorf("%w: unexpected plan schema %q", ErrRunnerInfrastructure, header.SchemaVersion)
	}
	extraArgs := []string{}
	if confirmToken != "" {
		extraArgs = append(extraArgs, "--confirm-token", confirmToken)
	}
	if authorityStateDirectory != "" {
		extraArgs = append(extraArgs, "--authority-state-dir", authorityStateDirectory)
	}
	return r.runPayload(ctx, plan, header.Mode, extraArgs)
}

// RunV3Reconcile runs only a read-only exact-name platform query. It does not
// pass a confirmation token and the runner cannot enter the submit workflow.
func (r Runner) RunV3Reconcile(ctx context.Context, plan json.RawMessage) (RpaResult, error) {
	return r.runPayload(ctx, plan, "prepare", []string{"--reconcile-only"})
}

func (r Runner) runPayload(ctx context.Context, payload []byte, mode string, extraArgs []string) (RpaResult, error) {
	timeout := r.PrepareTimeout
	if mode == "submit" {
		timeout = r.SubmitTimeout
	}
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if len(r.Command) == 0 {
		return RpaResult{}, fmt.Errorf("%w: runner command is not configured", ErrRunnerInfrastructure)
	}
	args := append(append([]string{}, r.Command[1:]...), r.ScriptPath)
	resultFile, err := os.CreateTemp("", "cookies-browser-rpa-result-*.json")
	if err != nil {
		return RpaResult{}, fmt.Errorf("%w: create result file: %v", ErrRunnerInfrastructure, err)
	}
	resultPath := resultFile.Name()
	_ = resultFile.Close()
	defer os.Remove(resultPath)
	if r.EdgeSessionFile != "" {
		args = append(args, "--session-file", r.EdgeSessionFile)
	} else {
		args = append(args, r.CDPEndpoint)
	}
	args = append(args, "--result-file", resultPath)
	args = append(args, extraArgs...)
	cmd := exec.CommandContext(ctx, r.Command[0], args...)
	cmd.Dir = r.WorkDir
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = limitedWriter{Buffer: &stderr, Cap: stderrCap}
	runErr := cmd.Run()

	resultPayload, readErr := os.ReadFile(resultPath)
	resultFileBytes := len(resultPayload)
	if readErr != nil || len(bytes.TrimSpace(resultPayload)) == 0 {
		resultPayload = stdout.Bytes()
	}
	result, parseErr := parseResult(resultPayload)
	if parseErr == nil {
		return result, nil
	}
	if runErr != nil {
		return RpaResult{}, fmt.Errorf("%w: %v: %s", ErrRunnerInfrastructure, runErr, tailMessage(stderr.String()))
	}
	return RpaResult{}, fmt.Errorf("%w: %v (result_file_bytes=%d stdout_bytes=%d result_read_error=%v): %s", ErrRunnerInfrastructure, parseErr, resultFileBytes, stdout.Len(), readErr, tailMessage(stderr.String()))
}

func parseResult(payload []byte) (RpaResult, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return RpaResult{}, errors.New("empty result")
	}
	var result RpaResult
	if err := json.Unmarshal(trimmed, &result); err == nil {
		if result.SchemaVersion != ResultSchemaV1 && result.SchemaVersion != ResultSchemaV2 {
			return RpaResult{}, fmt.Errorf("unexpected result schema %q", result.SchemaVersion)
		}
		return result, nil
	}
	// Tolerate third-party noise on stdout: recover the last line that looks
	// like the result document.
	for _, schema := range []string{ResultSchemaV2, ResultSchemaV1} {
		marker := []byte(`{"schema_version":"` + schema)
		if start := bytes.LastIndex(trimmed, marker); start >= 0 {
			var recovered RpaResult
			if err := json.Unmarshal(trimmed[start:], &recovered); err == nil {
				return recovered, nil
			}
		}
	}
	return RpaResult{}, fmt.Errorf("unparseable result document: %w", errJSON)
}

var errJSON = errors.New("result JSON did not parse")

func tailMessage(value string) string {
	const cap = 512
	if len(value) <= cap {
		return value
	}
	return value[len(value)-cap:]
}

type limitedWriter struct {
	Buffer *bytes.Buffer
	Cap    int
}

func (w limitedWriter) Write(p []byte) (int, error) {
	if w.Buffer.Len() >= w.Cap {
		return len(p), nil
	}
	room := w.Cap - w.Buffer.Len()
	if len(p) > room {
		p = p[:room]
	}
	return w.Buffer.Write(p)
}
