package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"os/exec"
	"sync"
	"time"
)

type deliveryFieldCapabilityReader struct {
	command     []string
	sessionFile string
	mu          sync.Mutex
}

func (r *deliveryFieldCapabilityReader) ReadFieldCapabilities(ctx context.Context, request delivery.FieldCapabilityRequest) (delivery.FieldCapabilitySnapshot, error) {
	if !r.mu.TryLock() {
		return delivery.FieldCapabilitySnapshot{}, fmt.Errorf("%w: another field inspection is running", delivery.ErrInvalidState)
	}
	defer r.mu.Unlock()
	if len(r.command) == 0 || r.sessionFile == "" {
		return delivery.FieldCapabilitySnapshot{}, delivery.ErrUnsupportedConfigurationWorkflow
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	args := append(append([]string{}, r.command[1:]...), "scripts/oceanengine-field-capabilities.ts", "--session-file", r.sessionFile)
	cmd := exec.CommandContext(ctx, r.command[0], args...)
	payload, err := json.Marshal(request)
	if err != nil {
		return delivery.FieldCapabilitySnapshot{}, err
	}
	cmd.Stdin = bytes.NewReader(payload)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return delivery.FieldCapabilitySnapshot{}, fmt.Errorf("%w: 无法读取平台字段状态，请确认 Edge 会话及账户登录后重试", delivery.ErrInvalidState)
	}
	var result delivery.FieldCapabilitySnapshot
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return result, fmt.Errorf("%w: invalid field observation", delivery.ErrInvalidState)
	}
	return result, nil
}
