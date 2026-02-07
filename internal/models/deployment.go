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
	SSHKeyPath string `json:"-"` // Not accepted from API — prevents arbitrary file read
	SSHPort    int    `json:"ssh_port"`
	Domain     string `json:"domain"`

	// SSL Configuration
	SSLMode          string `json:"ssl_mode"`
	SSLCert          string `json:"ssl_cert"`
	SSLKey           string `json:"ssl_key"`
	LetsEncryptEmail string `json:"letsencrypt_email"`

	// CloudStack Configuration
	CloudStackMode    string `json:"cloudstack_mode"`
	CloudStackVersion string `json:"cloudstack_version"`

	// ECR Token
	ECRToken string `json:"ecr_token"`
}

// DeploymentSummary contains only safe, non-sensitive fields for API responses.
type DeploymentSummary struct {
	ServerIP       string `json:"server_ip"`
	SSHUser        string `json:"ssh_user"`
	SSHPort        int    `json:"ssh_port"`
	Domain         string `json:"domain"`
	SSLMode        string `json:"ssl_mode"`
	CloudStackMode string `json:"cloudstack_mode"`
}

// NewSummary creates a safe summary from a deploy request (no secrets).
func NewSummary(req DeployRequest) DeploymentSummary {
	return DeploymentSummary{
		ServerIP:       req.ServerIP,
		SSHUser:        req.SSHUser,
		SSHPort:        req.SSHPort,
		Domain:         req.Domain,
		SSLMode:        req.SSLMode,
		CloudStackMode: req.CloudStackMode,
	}
}

type Stage struct {
	Name     string `json:"name"`
	MatchKey string `json:"-"`      // Used for log line matching; falls back to Name if empty
	Status   string `json:"status"` // "pending", "running", "done", "error"
}

type Deployment struct {
	ID           string            `json:"id"`
	Request      DeployRequest     `json:"-"`       // Internal only — never serialized
	Summary      DeploymentSummary `json:"config"`  // Safe subset for API
	Status       DeploymentStatus  `json:"status"`
	StartedAt    time.Time         `json:"started_at"`
	EndedAt      *time.Time        `json:"ended_at,omitempty"`
	Logs         []string          `json:"logs,omitempty"`
	Stages       []Stage           `json:"stages"`
	CurrentStage int               `json:"current_stage"`
}

// BuildStages returns the ordered list of deployment stages matching script execution order.
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
			Stage{Name: "Generating SSL Certificate", MatchKey: "Generating Let's Encrypt SSL Certificate", Status: "pending"},
			Stage{Name: "Setting up Certificate Renewal", MatchKey: "Setting up Automatic Certificate Renewal", Status: "pending"},
		)
	}
	stages = append(stages,
		Stage{Name: "Installing MariaDB", Status: "pending"},
		Stage{Name: "Installing MongoDB", Status: "pending"},
		Stage{Name: "Installing RabbitMQ", Status: "pending"},
		Stage{Name: "Setting up NFS", Status: "pending"},
	)
	stages = append(stages,
		Stage{Name: "Setting up Namespace", MatchKey: "Setting up Kubernetes Namespace", Status: "pending"},
		Stage{Name: "Setting up ECR Credentials", MatchKey: "Setting up AWS ECR Credentials", Status: "pending"},
		Stage{Name: "Setting up TLS Secret", Status: "pending"},
		Stage{Name: "Deploying StackBill", Status: "pending"},
		Stage{Name: "Setting up Istio Gateway", Status: "pending"},
		Stage{Name: "Waiting for Pods", MatchKey: "Waiting for StackBill Pods", Status: "pending"},
	)
	// CloudStack simulator runs AFTER pods are ready
	if req.CloudStackMode == "simulator" {
		stages = append(stages,
			Stage{Name: "Installing Podman", Status: "pending"},
			Stage{Name: "Deploying CloudStack Simulator", Status: "pending"},
			Stage{Name: "Configuring CloudStack", MatchKey: "Configuring CloudStack RabbitMQ", Status: "pending"},
			Stage{Name: "Creating CloudStack User", MatchKey: "Creating CloudStack Admin User for StackBill", Status: "pending"},
		)
	}
	stages = append(stages,
		Stage{Name: "Saving Credentials", Status: "pending"},
	)
	return stages
}
