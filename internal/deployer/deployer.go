package deployer

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/models"

	"golang.org/x/crypto/ssh"
)

type LogCallback func(line string)

type Deployer struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Deployer {
	return &Deployer{cfg: cfg}
}

func (d *Deployer) Deploy(req models.DeployRequest, onLog LogCallback) error {
	onLog("Connecting to server " + req.ServerIP + "...")

	client, err := d.connect(req)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	onLog("Connected successfully.")

	// Upload install script
	onLog("Uploading installation script...")
	if err := d.uploadScript(client); err != nil {
		return fmt.Errorf("script upload failed: %w", err)
	}
	onLog("Script uploaded.")

	// Build command with options
	cmd := d.buildCommand(req)
	onLog("Starting deployment: " + cmd)

	// Execute and stream output
	if err := d.executeAndStream(client, cmd, onLog); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	onLog("Deployment completed successfully!")
	return nil
}

func (d *Deployer) connect(req models.DeployRequest) (*ssh.Client, error) {
	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	var authMethods []ssh.AuthMethod

	// Password auth
	if req.SSHPass != "" {
		authMethods = append(authMethods, ssh.Password(req.SSHPass))
	}

	// Key-based auth
	if req.SSHKeyPath != "" {
		key, err := os.ReadFile(req.SSHKeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read SSH key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("unable to parse SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User:            req.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", req.ServerIP, sshPort)
	return ssh.Dial("tcp", addr, config)
}

func (d *Deployer) uploadScript(client *ssh.Client) error {
	// Get the script path
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	scriptPath := filepath.Join(projectRoot, d.cfg.ScriptPath)

	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("cannot read script: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Upload via stdin to cat
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		w.Write(scriptContent)
	}()

	return session.Run("cat > /tmp/install-stackbill-poc.sh && chmod +x /tmp/install-stackbill-poc.sh")
}

func (d *Deployer) buildCommand(req models.DeployRequest) string {
	args := []string{"sudo", "bash", "/tmp/install-stackbill-poc.sh"}

	if req.Domain != "" {
		args = append(args, "--domain", req.Domain)
	}
	if req.EnableCloudStack {
		args = append(args, "--cloudstack")
	}
	if req.CloudStackVersion != "" {
		args = append(args, "--cloudstack-version", req.CloudStackVersion)
	}
	if req.EnableSSL {
		args = append(args, "--ssl")
	}
	if req.EnableMonitoring {
		args = append(args, "--monitoring")
	}

	return strings.Join(args, " ")
}

func (d *Deployer) executeAndStream(client *ssh.Client, cmd string, onLog LogCallback) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Request PTY for colored output
	if err := session.RequestPty("xterm", 80, 200, ssh.TerminalModes{}); err != nil {
		log.Printf("PTY request failed (continuing without): %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	// Stream stdout
	go streamLines(stdout, onLog)
	// Stream stderr
	go streamLines(stderr, onLog)

	return session.Wait()
}

func streamLines(r io.Reader, onLog LogCallback) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		onLog(scanner.Text())
	}
}
