package provision

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LogFunc func(step, status, message string)

type AnsibleRunner struct {
	PlaybookDir string
}

func NewAnsibleRunner(playbookDir string) *AnsibleRunner {
	return &AnsibleRunner{PlaybookDir: playbookDir}
}

func (a *AnsibleRunner) Provision(ctx context.Context, ip, sshUser, sshPass, workerUUID, apiKey, dashboardURL, agentPort string, log LogFunc) error {
	inventoryFile, err := a.writeInventory(ip, sshUser, sshPass)
	if err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	defer os.Remove(inventoryFile)

	playbookPath := filepath.Join(a.PlaybookDir, "provision.yml")

	args := []string{
		"-i", inventoryFile,
		playbookPath,
		"--extra-vars", fmt.Sprintf(
			"worker_uuid=%s agent_api_key=%s dashboard_url=%s agent_port=%s",
			workerUUID, apiKey, dashboardURL, agentPort,
		),
	}

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...)
	cmd.Env = append(os.Environ(), "ANSIBLE_HOST_KEY_CHECKING=False")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ansible-playbook: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		step, status := parseAnsibleLine(line)
		log(step, status, line)
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		line := errScanner.Text()
		log("ansible", "error", line)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ansible-playbook failed: %w", err)
	}
	return nil
}

func (a *AnsibleRunner) writeInventory(ip, sshUser, sshPass string) (string, error) {
	content := fmt.Sprintf(
		"[workers]\n%s ansible_user=%s ansible_password=%s ansible_become=yes ansible_become_method=sudo\n",
		ip, sshUser, sshPass,
	)
	f, err := os.CreateTemp("", "ansible-inventory-*.ini")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func parseAnsibleLine(line string) (step, status string) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "TASK ["):
		end := strings.Index(line, "]")
		if end > 6 {
			return line[6:end], "running"
		}
	case strings.HasPrefix(line, "ok:"):
		return "task", "ok"
	case strings.HasPrefix(line, "changed:"):
		return "task", "changed"
	case strings.HasPrefix(line, "failed:"):
		return "task", "failed"
	case strings.HasPrefix(line, "PLAY RECAP"):
		return "recap", "info"
	}
	return "output", "info"
}
