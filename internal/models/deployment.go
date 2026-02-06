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

	// Installation options
	EnableCloudStack        bool   `json:"enable_cloudstack"`
	CloudStackVersion       string `json:"cloudstack_version"`
	EnableSSL               bool   `json:"enable_ssl"`
	EnableMonitoring        bool   `json:"enable_monitoring"`
}

type Deployment struct {
	ID        string           `json:"id"`
	Request   DeployRequest    `json:"request"`
	Status    DeploymentStatus `json:"status"`
	StartedAt time.Time        `json:"started_at"`
	EndedAt   *time.Time       `json:"ended_at,omitempty"`
	Logs      []string         `json:"logs,omitempty"`
}
