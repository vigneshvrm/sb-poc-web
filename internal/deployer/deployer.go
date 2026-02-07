package deployer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/models"

	"golang.org/x/crypto/ssh"
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
	onLog("Starting deployment...")

	// Pass sudo password separately via stdin (never appears in process list)
	var sudoPass string
	if req.SSHUser != "root" {
		sudoPass = req.SSHPass
	}

	// Execute and stream output
	execErr := d.executeAndStream(client, cmd, sudoPass, onLog)

	// Clean up: remove script from remote server
	onLog("Cleaning up installation script...")
	d.removeScript(client)

	if execErr != nil {
		return fmt.Errorf("deployment failed: %w", execErr)
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

// shellQuote safely wraps a string in single quotes for shell execution.
// This prevents command injection by ensuring special characters are not interpreted.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (d *Deployer) buildCommand(req models.DeployRequest) string {
	args := []string{"bash", "/tmp/install-stackbill-poc.sh"}

	args = append(args, "--domain", shellQuote(req.Domain))
	args = append(args, "--yes")

	// SSL
	if req.SSLMode == "letsencrypt" {
		args = append(args, "--letsencrypt")
		if req.LetsEncryptEmail != "" {
			args = append(args, "--email", shellQuote(req.LetsEncryptEmail))
		}
	} else if req.SSLMode == "custom" {
		args = append(args, "--ssl-cert", shellQuote(req.SSLCert), "--ssl-key", shellQuote(req.SSLKey))
	}

	// CloudStack
	if req.CloudStackMode != "" {
		args = append(args, "--cloudstack-mode", shellQuote(req.CloudStackMode))
		if req.CloudStackMode == "simulator" && req.CloudStackVersion != "" {
			args = append(args, "--cloudstack-version", shellQuote(req.CloudStackVersion))
		}
	}

	// ECR Token
	if req.ECRToken != "" {
		args = append(args, "--ecr-token", shellQuote(req.ECRToken))
	}

	cmd := strings.Join(args, " ")

	// Non-root: use sudo -S which reads password from stdin (piped separately)
	if req.SSHUser != "root" {
		cmd = "sudo -S " + cmd
	}

	return cmd
}

func (d *Deployer) executeAndStream(client *ssh.Client, cmd string, sudoPass string, onLog LogCallback) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	// Pipe sudo password via stdin — never visible in process list or ps output
	if sudoPass != "" {
		stdinPipe, err := session.StdinPipe()
		if err != nil {
			return err
		}
		go func() {
			defer stdinPipe.Close()
			io.WriteString(stdinPipe, sudoPass+"\n")
		}()
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

func (d *Deployer) removeScript(client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()
	session.Run("rm -f /tmp/install-stackbill-poc.sh")
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
