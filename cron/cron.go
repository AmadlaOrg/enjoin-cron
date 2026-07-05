package cron

import (
	"fmt"
	"os/exec"
	"strings"
)

// Config represents the cron entity input.
type Config struct {
	Jobs    []JobSpec `json:"jobs" yaml:"jobs"`
	Backend string    `json:"backend,omitempty" yaml:"backend,omitempty"` // "crontab" or "systemd-timer"
}

// JobSpec defines a scheduled job.
type JobSpec struct {
	Name             string            `json:"name" yaml:"name"`
	Schedule         string            `json:"schedule" yaml:"schedule"`
	Command          string            `json:"command" yaml:"command"`
	User             string            `json:"user,omitempty" yaml:"user,omitempty"`
	Environment      map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty" yaml:"working_directory,omitempty"`
	LogOutput        string            `json:"log_output,omitempty" yaml:"log_output,omitempty"`
	LogError         string            `json:"log_error,omitempty" yaml:"log_error,omitempty"`
	OnFailure        string            `json:"on_failure,omitempty" yaml:"on_failure,omitempty"`
	Timeout          string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	State            string            `json:"state,omitempty" yaml:"state,omitempty"` // "present" or "absent"
}

// Result represents the outcome of an apply or validate operation.
type Result struct {
	Success bool           `json:"success"`
	Status  string         `json:"status"`
	Changes []ChangeDetail `json:"changes,omitempty"`
	Errors  []string       `json:"errors,omitempty"`
}

// ChangeDetail describes a single change made or planned.
type ChangeDetail struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Name     string `json:"name"`
	Detail   string `json:"detail,omitempty"`
}

type backendType int

const (
	backendCrontab backendType = iota
	backendSystemdTimer
)

var (
	execCommand    = exec.Command
	detectBackend  = detectCronBackend
	writeFileFunc  = writeFile
)

func detectCronBackend() backendType {
	if path, err := exec.LookPath("systemctl"); err == nil && path != "" {
		return backendSystemdTimer
	}
	return backendCrontab
}

// writeFile is a helper wrapping os.WriteFile for mocking.
func writeFile(name string, data []byte, perm uint32) error {
	cmd := execCommand("tee", name)
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func jobUser(j JobSpec) string {
	if j.User != "" {
		return j.User
	}
	return "root"
}

func jobState(j JobSpec) string {
	if j.State != "" {
		return j.State
	}
	return "present"
}

// Apply creates or removes cron jobs / systemd timers.
func Apply(cfg *Config) *Result {
	result := &Result{Success: true, Status: "applied"}
	b := resolveBackend(cfg.Backend)

	for _, job := range cfg.Jobs {
		state := jobState(job)
		var change *ChangeDetail
		var err error

		switch b {
		case backendCrontab:
			if state == "absent" {
				change, err = removeCrontabEntry(job)
			} else {
				change, err = applyCrontabEntry(job)
			}
		case backendSystemdTimer:
			if state == "absent" {
				change, err = removeSystemdTimer(job)
			} else {
				change, err = applySystemdTimer(job)
			}
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("job %s: %v", job.Name, err))
			result.Success = false
			continue
		}
		if change != nil {
			result.Changes = append(result.Changes, *change)
		}
	}

	if !result.Success {
		result.Status = "failed"
	}
	return result
}

// Validate checks the configuration without making changes.
func Validate(cfg *Config) *Result {
	result := &Result{Success: true, Status: "valid"}

	if len(cfg.Jobs) == 0 {
		result.Errors = append(result.Errors, "jobs: at least one job is required")
		result.Success = false
		result.Status = "invalid"
		return result
	}

	for i, job := range cfg.Jobs {
		if job.Name == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("job %d: name required", i))
			result.Success = false
		}
		if job.Schedule == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("job %d: schedule required", i))
			result.Success = false
		}
		if job.Command == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("job %d: command required", i))
			result.Success = false
		}
		if job.State != "" && job.State != "present" && job.State != "absent" {
			result.Errors = append(result.Errors, fmt.Sprintf("job %d: state must be 'present' or 'absent'", i))
			result.Success = false
		}

		action := "would_add"
		if jobState(job) == "absent" {
			action = "would_remove"
		}
		result.Changes = append(result.Changes, ChangeDetail{
			Action:   action,
			Resource: "cron_job",
			Name:     job.Name,
		})
	}

	if !result.Success {
		result.Status = "invalid"
	}
	return result
}

func resolveBackend(configured string) backendType {
	switch configured {
	case "crontab":
		return backendCrontab
	case "systemd-timer":
		return backendSystemdTimer
	default:
		return detectBackend()
	}
}

// buildCrontabLine constructs a crontab entry line with optional environment and redirection.
func buildCrontabLine(job JobSpec) string {
	var parts []string

	// Add environment variables as inline env
	for k, v := range job.Environment {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}

	cmd := job.Command
	if job.WorkingDirectory != "" {
		cmd = fmt.Sprintf("cd %s && %s", job.WorkingDirectory, cmd)
	}
	if job.LogOutput != "" {
		cmd = fmt.Sprintf("%s >> %s", cmd, job.LogOutput)
	}
	if job.LogError != "" {
		cmd = fmt.Sprintf("%s 2>> %s", cmd, job.LogError)
	}

	envPrefix := ""
	if len(parts) > 0 {
		envPrefix = strings.Join(parts, " ") + " "
	}

	return fmt.Sprintf("%s %s%s # enjoin:%s", job.Schedule, envPrefix, cmd, job.Name)
}

func applyCrontabEntry(job JobSpec) (*ChangeDetail, error) {
	u := jobUser(job)

	// Read existing crontab
	readCmd := execCommand("crontab", "-l", "-u", u)
	existingOutput, _ := readCmd.CombinedOutput()
	existing := string(existingOutput)

	// Check if job already exists by marker
	marker := fmt.Sprintf("# enjoin:%s", job.Name)
	newLine := buildCrontabLine(job)

	var lines []string
	found := false
	for _, line := range strings.Split(existing, "\n") {
		if strings.Contains(line, marker) {
			lines = append(lines, newLine)
			found = true
		} else {
			lines = append(lines, line)
		}
	}
	if !found {
		// Remove trailing empty line before appending
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, newLine)
		lines = append(lines, "") // trailing newline
	}

	content := strings.Join(lines, "\n")
	writeCmd := execCommand("crontab", "-u", u, "-")
	writeCmd.Stdin = strings.NewReader(content)
	if out, err := writeCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("crontab write failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	action := "created"
	if found {
		action = "updated"
	}
	return &ChangeDetail{
		Action:   action,
		Resource: "cron_job",
		Name:     job.Name,
		Detail:   fmt.Sprintf("backend=crontab user=%s", u),
	}, nil
}

func removeCrontabEntry(job JobSpec) (*ChangeDetail, error) {
	u := jobUser(job)

	readCmd := execCommand("crontab", "-l", "-u", u)
	existingOutput, _ := readCmd.CombinedOutput()
	existing := string(existingOutput)

	marker := fmt.Sprintf("# enjoin:%s", job.Name)
	var lines []string
	found := false
	for _, line := range strings.Split(existing, "\n") {
		if strings.Contains(line, marker) {
			found = true
			continue
		}
		lines = append(lines, line)
	}

	if !found {
		return &ChangeDetail{
			Action:   "absent",
			Resource: "cron_job",
			Name:     job.Name,
			Detail:   "already absent",
		}, nil
	}

	content := strings.Join(lines, "\n")
	writeCmd := execCommand("crontab", "-u", u, "-")
	writeCmd.Stdin = strings.NewReader(content)
	if out, err := writeCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("crontab write failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return &ChangeDetail{
		Action:   "removed",
		Resource: "cron_job",
		Name:     job.Name,
		Detail:   fmt.Sprintf("backend=crontab user=%s", u),
	}, nil
}

func buildServiceUnit(job JobSpec) string {
	var sb strings.Builder
	sb.WriteString("[Unit]\n")
	sb.WriteString(fmt.Sprintf("Description=Enjoin cron job: %s\n", job.Name))
	sb.WriteString("\n[Service]\n")
	sb.WriteString("Type=oneshot\n")
	sb.WriteString(fmt.Sprintf("ExecStart=%s\n", job.Command))
	if job.User != "" {
		sb.WriteString(fmt.Sprintf("User=%s\n", job.User))
	}
	if job.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", job.WorkingDirectory))
	}
	for k, v := range job.Environment {
		sb.WriteString(fmt.Sprintf("Environment=%s=%s\n", k, v))
	}
	if job.LogOutput != "" {
		sb.WriteString(fmt.Sprintf("StandardOutput=append:%s\n", job.LogOutput))
	}
	if job.LogError != "" {
		sb.WriteString(fmt.Sprintf("StandardError=append:%s\n", job.LogError))
	}
	if job.Timeout != "" {
		sb.WriteString(fmt.Sprintf("TimeoutStartSec=%s\n", job.Timeout))
	}
	return sb.String()
}

func buildTimerUnit(job JobSpec) string {
	var sb strings.Builder
	sb.WriteString("[Unit]\n")
	sb.WriteString(fmt.Sprintf("Description=Timer for enjoin cron job: %s\n", job.Name))
	sb.WriteString("\n[Timer]\n")
	sb.WriteString(fmt.Sprintf("OnCalendar=%s\n", job.Schedule))
	sb.WriteString("Persistent=true\n")
	sb.WriteString("\n[Install]\n")
	sb.WriteString("WantedBy=timers.target\n")
	return sb.String()
}

func unitName(job JobSpec) string {
	return fmt.Sprintf("enjoin-%s", job.Name)
}

func applySystemdTimer(job JobSpec) (*ChangeDetail, error) {
	name := unitName(job)
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", name)
	timerPath := fmt.Sprintf("/etc/systemd/system/%s.timer", name)

	// Write service unit
	if err := writeFileFunc(servicePath, []byte(buildServiceUnit(job)), 0644); err != nil {
		return nil, fmt.Errorf("write service unit: %w", err)
	}

	// Write timer unit
	if err := writeFileFunc(timerPath, []byte(buildTimerUnit(job)), 0644); err != nil {
		return nil, fmt.Errorf("write timer unit: %w", err)
	}

	// Reload systemd
	reloadCmd := execCommand("systemctl", "daemon-reload")
	if out, err := reloadCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("daemon-reload failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Enable and start timer
	enableCmd := execCommand("systemctl", "enable", "--now", name+".timer")
	if out, err := enableCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("enable timer failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return &ChangeDetail{
		Action:   "created",
		Resource: "systemd_timer",
		Name:     job.Name,
		Detail:   fmt.Sprintf("backend=systemd-timer unit=%s", name),
	}, nil
}

func removeSystemdTimer(job JobSpec) (*ChangeDetail, error) {
	name := unitName(job)

	// Stop and disable timer
	stopCmd := execCommand("systemctl", "disable", "--now", name+".timer")
	if out, err := stopCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("disable timer failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Remove unit files
	rmCmd := execCommand("rm", "-f",
		fmt.Sprintf("/etc/systemd/system/%s.service", name),
		fmt.Sprintf("/etc/systemd/system/%s.timer", name),
	)
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("remove unit files failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Reload systemd
	reloadCmd := execCommand("systemctl", "daemon-reload")
	if out, err := reloadCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("daemon-reload failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return &ChangeDetail{
		Action:   "removed",
		Resource: "systemd_timer",
		Name:     job.Name,
		Detail:   fmt.Sprintf("backend=systemd-timer unit=%s", name),
	}, nil
}
