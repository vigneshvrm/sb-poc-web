package models

import "time"

type DeploymentStatus string

const (
	StatusPending  DeploymentStatus = "pending"
	StatusRunning  DeploymentStatus = "running"
	StatusSuccess  DeploymentStatus = "success"
	StatusFailed   DeploymentStatus = "failed"
)

type DeployRequest struct {
	ServerIP   string `json:"server_ip"`
	SSHUser    string `json:"ssh_user"`
	SSHPass    string `json:"ssh_pass"`
	SSHKeyPath string `json:"ssh_key_path"`
	SSHPort    int    `json:"ssh_port"`
	Domain     string `json:"domain"`

	// SSL Configuration
	SSLMode          string `json:"ssl_mode"`          // "letsencrypt" or "custom"
	SSLCert          string `json:"ssl_cert"`           // cert path on remote server (custom mode)
	SSLKey           string `json:"ssl_key"`            // key path on remote server (custom mode)
	LetsEncryptEmail string `json:"letsencrypt_email"`  // email (letsencrypt mode)

	// CloudStack Configuration
	CloudStackMode    string `json:"cloudstack_mode"`    // "existing" or "simulator"
	CloudStackVersion string `json:"cloudstack_version"` // e.g. "4.21.0.0"

	// ECR Token
	ECRToken string `json:"ecr_token"`
}

type Stage struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pending", "running", "done", "error"
}

type Deployment struct {
	ID           string           `json:"id"`
	Request      DeployRequest    `json:"request"`
	Status       DeploymentStatus `json:"status"`
	StartedAt    time.Time        `json:"started_at"`
	EndedAt      *time.Time       `json:"ended_at,omitempty"`
	Logs         []string         `json:"logs,omitempty"`
	Stages       []Stage          `json:"stages"`
	CurrentStage int              `json:"current_stage"`
}

// BuildStages returns the ordered list of deployment stages based on request config.
func BuildStages(req DeployRequest) []Stage {
	stages := []Stage{
		{Name: "Checking System Requirements", Status: "pending"},
		{Name: "Installing K3s", Status: "pending"},
		{Name: "Installing Helm", Status: "pending"},
		{Name: "Installing Istio", Status: "pending"},
	}
	if req.SSLMode == "letsencrypt" {
		stages = append(stages,
			Stage{Name: "Installing Certbot", Status: "pending"},
			Stage{Name: "Generating SSL Certificate", Status: "pending"},
		)
	}
	stages = append(stages,
		Stage{Name: "Installing MariaDB", Status: "pending"},
		Stage{Name: "Installing MongoDB", Status: "pending"},
		Stage{Name: "Installing RabbitMQ", Status: "pending"},
		Stage{Name: "Setting up NFS", Status: "pending"},
		Stage{Name: "Setting up Namespace", Status: "pending"},
		Stage{Name: "Setting up ECR Credentials", Status: "pending"},
		Stage{Name: "Setting up TLS Secret", Status: "pending"},
		Stage{Name: "Deploying StackBill", Status: "pending"},
		Stage{Name: "Setting up Istio Gateway", Status: "pending"},
		Stage{Name: "Waiting for Pods", Status: "pending"},
	)
	if req.CloudStackMode == "simulator" {
		stages = append(stages,
			Stage{Name: "Installing Podman", Status: "pending"},
			Stage{Name: "Deploying CloudStack Simulator", Status: "pending"},
			Stage{Name: "Configuring CloudStack", Status: "pending"},
			Stage{Name: "Creating CloudStack User", Status: "pending"},
		)
	}
	stages = append(stages, Stage{Name: "Saving Credentials", Status: "pending"})
	return stages
}
