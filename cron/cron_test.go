package cron

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_ValidJobs(t *testing.T) {
	cfg := &Config{
		Jobs: []JobSpec{
			{Name: "backup", Schedule: "0 2 * * *", Command: "/usr/local/bin/backup.sh"},
			{Name: "cleanup", Schedule: "0 4 * * 0", Command: "/usr/local/bin/cleanup.sh", User: "www-data"},
		},
	}

	result := Validate(cfg)
	assert.True(t, result.Success)
	assert.Equal(t, "valid", result.Status)
	assert.Len(t, result.Changes, 2)
	assert.Equal(t, "would_add", result.Changes[0].Action)
	assert.Equal(t, "cron_job", result.Changes[0].Resource)
	assert.Equal(t, "would_add", result.Changes[1].Action)
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	cfg := &Config{
		Jobs: []JobSpec{
			{Schedule: "0 2 * * *", Command: "/usr/local/bin/backup.sh"}, // missing name
			{Name: "cleanup", Command: "/usr/local/bin/cleanup.sh"},      // missing schedule
			{Name: "report", Schedule: "0 6 * * *"},                      // missing command
		},
	}

	result := Validate(cfg)
	assert.False(t, result.Success)
	assert.Equal(t, "invalid", result.Status)
	assert.Len(t, result.Errors, 3) // 1 missing name + 1 missing schedule + 1 missing command
}

func TestValidate_InvalidState(t *testing.T) {
	cfg := &Config{
		Jobs: []JobSpec{
			{Name: "bad", Schedule: "* * * * *", Command: "/bin/true", State: "invalid"},
		},
	}

	result := Validate(cfg)
	assert.False(t, result.Success)
	assert.Equal(t, "invalid", result.Status)
	assert.Contains(t, result.Errors[0], "state must be")
}

func TestValidate_EmptyJobs(t *testing.T) {
	cfg := &Config{
		Jobs: []JobSpec{},
	}

	result := Validate(cfg)
	assert.False(t, result.Success)
	assert.Equal(t, "invalid", result.Status)
	assert.Contains(t, result.Errors[0], "at least one job is required")
}

func TestValidate_AbsentState(t *testing.T) {
	cfg := &Config{
		Jobs: []JobSpec{
			{Name: "old-job", Schedule: "0 0 * * *", Command: "/bin/true", State: "absent"},
		},
	}

	result := Validate(cfg)
	assert.True(t, result.Success)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "would_remove", result.Changes[0].Action)
}

func TestApply_CrontabBackend(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendCrontab }

	origCmd := execCommand
	defer func() { execCommand = origCmd }()
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	cfg := &Config{
		Backend: "crontab",
		Jobs: []JobSpec{
			{Name: "backup", Schedule: "0 2 * * *", Command: "/usr/local/bin/backup.sh"},
		},
	}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Equal(t, "applied", result.Status)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "cron_job", result.Changes[0].Resource)
	assert.Equal(t, "backup", result.Changes[0].Name)
}

func TestApply_SystemdTimerBackend(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendSystemdTimer }

	origCmd := execCommand
	defer func() { execCommand = origCmd }()
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	origWrite := writeFileFunc
	defer func() { writeFileFunc = origWrite }()
	writeFileFunc = func(name string, data []byte, perm uint32) error {
		return nil
	}

	cfg := &Config{
		Backend: "systemd-timer",
		Jobs: []JobSpec{
			{
				Name:     "report",
				Schedule: "*-*-* 06:00:00",
				Command:  "/usr/local/bin/report.sh",
				User:     "app",
			},
		},
	}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Equal(t, "applied", result.Status)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "created", result.Changes[0].Action)
	assert.Equal(t, "systemd_timer", result.Changes[0].Resource)
	assert.Equal(t, "report", result.Changes[0].Name)
}

func TestApply_AbsentCrontab(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendCrontab }

	origCmd := execCommand
	defer func() { execCommand = origCmd }()
	execCommand = func(name string, arg ...string) *exec.Cmd {
		// crontab -l returns empty, crontab -u <user> - succeeds
		return exec.Command("echo", "")
	}

	cfg := &Config{
		Backend: "crontab",
		Jobs: []JobSpec{
			{Name: "old-job", Schedule: "0 0 * * *", Command: "/bin/true", State: "absent"},
		},
	}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "absent", result.Changes[0].Action)
	assert.Contains(t, result.Changes[0].Detail, "already absent")
}

func TestApply_AbsentSystemdTimer(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendSystemdTimer }

	origCmd := execCommand
	defer func() { execCommand = origCmd }()
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	cfg := &Config{
		Backend: "systemd-timer",
		Jobs: []JobSpec{
			{Name: "old-timer", Schedule: "0 0 * * *", Command: "/bin/true", State: "absent"},
		},
	}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "removed", result.Changes[0].Action)
	assert.Equal(t, "systemd_timer", result.Changes[0].Resource)
}

func TestApply_NoJobs(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendCrontab }

	cfg := &Config{}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Equal(t, "applied", result.Status)
	assert.Empty(t, result.Changes)
}

func TestApply_MultipleJobs(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendCrontab }

	origCmd := execCommand
	defer func() { execCommand = origCmd }()
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	cfg := &Config{
		Backend: "crontab",
		Jobs: []JobSpec{
			{Name: "backup", Schedule: "0 2 * * *", Command: "/usr/local/bin/backup.sh"},
			{Name: "cleanup", Schedule: "0 4 * * 0", Command: "/usr/local/bin/cleanup.sh", User: "www-data"},
		},
	}

	result := Apply(cfg)
	assert.True(t, result.Success)
	assert.Len(t, result.Changes, 2)
}

func TestDetectCronBackend(t *testing.T) {
	// Just verify the function runs without panic
	b := detectCronBackend()
	assert.True(t, b == backendCrontab || b == backendSystemdTimer)
}

func TestBuildCrontabLine(t *testing.T) {
	job := JobSpec{
		Name:     "backup",
		Schedule: "0 2 * * *",
		Command:  "/usr/local/bin/backup.sh",
	}

	line := buildCrontabLine(job)
	assert.Contains(t, line, "0 2 * * *")
	assert.Contains(t, line, "/usr/local/bin/backup.sh")
	assert.Contains(t, line, "# enjoin:backup")
}

func TestBuildCrontabLine_WithOptions(t *testing.T) {
	job := JobSpec{
		Name:             "report",
		Schedule:         "0 6 * * 1",
		Command:          "/usr/local/bin/report.sh",
		WorkingDirectory: "/opt/app",
		LogOutput:        "/var/log/report.log",
		LogError:         "/var/log/report.err",
	}

	line := buildCrontabLine(job)
	assert.Contains(t, line, "cd /opt/app && /usr/local/bin/report.sh")
	assert.Contains(t, line, ">> /var/log/report.log")
	assert.Contains(t, line, "2>> /var/log/report.err")
	assert.Contains(t, line, "# enjoin:report")
}

func TestBuildServiceUnit(t *testing.T) {
	job := JobSpec{
		Name:             "backup",
		Command:          "/usr/local/bin/backup.sh",
		User:             "app",
		WorkingDirectory: "/opt/app",
		Timeout:          "300",
	}

	unit := buildServiceUnit(job)
	assert.Contains(t, unit, "[Unit]")
	assert.Contains(t, unit, "[Service]")
	assert.Contains(t, unit, "Type=oneshot")
	assert.Contains(t, unit, "ExecStart=/usr/local/bin/backup.sh")
	assert.Contains(t, unit, "User=app")
	assert.Contains(t, unit, "WorkingDirectory=/opt/app")
	assert.Contains(t, unit, "TimeoutStartSec=300")
}

func TestBuildTimerUnit(t *testing.T) {
	job := JobSpec{
		Name:     "backup",
		Schedule: "*-*-* 02:00:00",
	}

	unit := buildTimerUnit(job)
	assert.Contains(t, unit, "[Unit]")
	assert.Contains(t, unit, "[Timer]")
	assert.Contains(t, unit, "OnCalendar=*-*-* 02:00:00")
	assert.Contains(t, unit, "Persistent=true")
	assert.Contains(t, unit, "[Install]")
	assert.Contains(t, unit, "WantedBy=timers.target")
}

func TestResolveBackend_Crontab(t *testing.T) {
	b := resolveBackend("crontab")
	assert.Equal(t, backendCrontab, b)
}

func TestResolveBackend_SystemdTimer(t *testing.T) {
	b := resolveBackend("systemd-timer")
	assert.Equal(t, backendSystemdTimer, b)
}

func TestResolveBackend_AutoDetect(t *testing.T) {
	origBackend := detectBackend
	defer func() { detectBackend = origBackend }()
	detectBackend = func() backendType { return backendCrontab }

	b := resolveBackend("")
	assert.Equal(t, backendCrontab, b)
}

func TestJobUser_Default(t *testing.T) {
	job := JobSpec{Name: "test"}
	assert.Equal(t, "root", jobUser(job))
}

func TestJobUser_Custom(t *testing.T) {
	job := JobSpec{Name: "test", User: "app"}
	assert.Equal(t, "app", jobUser(job))
}

func TestJobState_Default(t *testing.T) {
	job := JobSpec{Name: "test"}
	assert.Equal(t, "present", jobState(job))
}

func TestJobState_Absent(t *testing.T) {
	job := JobSpec{Name: "test", State: "absent"}
	assert.Equal(t, "absent", jobState(job))
}

func TestUnitName(t *testing.T) {
	job := JobSpec{Name: "my-backup"}
	assert.Equal(t, "enjoin-my-backup", unitName(job))
}
