package rparunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
)

type sessionProbeRunner struct {
	Command     []string
	ScriptPath  string
	WorkDir     string
	SessionFile string
	Timeout     time.Duration
}

func (r sessionProbeRunner) Run(ctx context.Context, accountID string) (browserautomation.EdgeSessionProbe, error) {
	if len(r.Command) == 0 || r.ScriptPath == "" || r.SessionFile == "" {
		return browserautomation.EdgeSessionProbe{}, fmt.Errorf("%w: Edge session probe is not configured", browserautomation.ErrEnvironmentUnavailable)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, _ := json.Marshal(map[string]string{"account_id": accountID})
	args := append(append([]string{}, r.Command[1:]...), r.ScriptPath, "--session-file", r.SessionFile)
	cmd := exec.CommandContext(ctx, r.Command[0], args...)
	cmd.Dir = r.WorkDir
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = limitedWriter{Buffer: &stderr, Cap: stderrCap}
	if err := cmd.Run(); err != nil {
		return browserautomation.EdgeSessionProbe{}, fmt.Errorf("%w: probe process failed: %v: %s", browserautomation.ErrEnvironmentUnavailable, err, tailMessage(stderr.String()))
	}
	var result browserautomation.EdgeSessionProbe
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return browserautomation.EdgeSessionProbe{}, fmt.Errorf("%w: invalid probe result", browserautomation.ErrEnvironmentUnavailable)
	}
	if result.SchemaVersion != browserautomation.EdgeSessionProbeSchemaV1 || (result.Status != "ready" && result.Status != "blocked") {
		return browserautomation.EdgeSessionProbe{}, fmt.Errorf("%w: invalid probe contract", browserautomation.ErrEnvironmentUnavailable)
	}
	return result, nil
}
