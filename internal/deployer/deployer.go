package deployer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/models"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?(\x07|\x1b\\)`)

type LogCallback func(line string)

type Deployer struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Deployer {
	return &Deployer{cfg: cfg}
}

func (d *Deployer) Deploy(req models.DeployRequest, onLog LogCallback) error {
	onLog("Preparing Ansible deployment to " + req.ServerIP + "...")

	// Create temp directory for inventory and vars (cleaned up after)
	tmpDir, err := os.MkdirTemp("", "sb-deploy-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write dynamic inventory
	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err := d.writeInventory(inventoryPath, req); err != nil {
		return fmt.Errorf("failed to write inventory: %w", err)
	}

	// Write extra vars JSON
	varsPath := filepath.Join(tmpDir, "vars.json")
	if err := d.writeVars(varsPath, req); err != nil {
		return fmt.Errorf("failed to write vars: %w", err)
	}

	// Get playbook path
	playbookPath := d.getPlaybookPath()
	ansibleCfgPath := d.getAnsibleCfgPath()

	onLog("Starting Ansible playbook...")

	// Build and run ansible-playbook command
	args := []string{
		playbookPath,
		"-i", inventoryPath,
		"--extra-vars", "@" + varsPath,
	}

	cmd := exec.Command("ansible-playbook", args...)
	cmd.Env = append(os.Environ(),
		"ANSIBLE_CONFIG="+ansibleCfgPath,
		"ANSIBLE_NOCOLOR=1",
		"ANSIBLE_FORCE_COLOR=0",
		"ANSIBLE_HOST_KEY_CHECKING=False",
	)

	// Pipe stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ansible-playbook: %w", err)
	}

	// Stream output
	done := make(chan struct{}, 2)
	go func() { streamLines(stdout, onLog); done <- struct{}{} }()
	go func() { streamLines(stderr, onLog); done <- struct{}{} }()

	// Wait for both streams to finish
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	onLog("Deployment completed successfully!")
	return nil
}

// writeInventory creates a temporary Ansible inventory file with target host details.
func (d *Deployer) writeInventory(path string, req models.DeployRequest) error {
	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	become := "yes"
	becomePass := req.SSHPass
	if req.SSHUser == "root" {
		become = "no"
		becomePass = ""
	}

	var sb strings.Builder
	sb.WriteString("[target]\n")
	sb.WriteString(fmt.Sprintf("%s ansible_user=%s ansible_ssh_pass=%s ansible_port=%d ansible_become=%s",
		req.ServerIP, req.SSHUser, req.SSHPass, sshPort, become))

	if becomePass != "" {
		sb.WriteString(fmt.Sprintf(" ansible_become_pass=%s", becomePass))
	}
	sb.WriteString("\n")

	return os.WriteFile(path, []byte(sb.String()), 0600)
}

// writeVars creates a temporary JSON file with deployment variables.
func (d *Deployer) writeVars(path string, req models.DeployRequest) error {
	vars := map[string]string{
		"domain":          req.Domain,
		"ssl_mode":        req.SSLMode,
		"cloudstack_mode": req.CloudStackMode,
		"ecr_token":       req.ECRToken,
	}

	if req.SSLMode == "letsencrypt" && req.LetsEncryptEmail != "" {
		vars["letsencrypt_email"] = req.LetsEncryptEmail
	}
	if req.SSLMode == "custom" {
		vars["ssl_cert_content"] = req.SSLCert
		vars["ssl_key_content"] = req.SSLKey
	}
	if req.CloudStackMode == "simulator" && req.CloudStackVersion != "" {
		vars["cloudstack_version"] = req.CloudStackVersion
	}

	data, err := json.Marshal(vars)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// getPlaybookPath returns the absolute path to the Ansible playbook.
func (d *Deployer) getPlaybookPath() string {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	return filepath.Join(projectRoot, d.cfg.AnsibleDir, "playbook.yml")
}

// getAnsibleCfgPath returns the absolute path to ansible.cfg.
func (d *Deployer) getAnsibleCfgPath() string {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	return filepath.Join(projectRoot, d.cfg.AnsibleDir, "ansible.cfg")
}

func streamLines(r io.Reader, onLog LogCallback) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := ansiRegex.ReplaceAllString(scanner.Text(), "")
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		onLog(line)
	}
}
