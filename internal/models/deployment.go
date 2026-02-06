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

type Deployment struct {
	ID        string           `json:"id"`
	Request   DeployRequest    `json:"request"`
	Status    DeploymentStatus `json:"status"`
	StartedAt time.Time        `json:"started_at"`
	EndedAt   *time.Time       `json:"ended_at,omitempty"`
	Logs      []string         `json:"logs,omitempty"`
}
