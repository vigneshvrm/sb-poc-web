package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	Port       string
	AnsibleDir string
	AuthToken  string
	TLSCert    string
	TLSKey     string
}

func Load() *Config {
	port := os.Getenv("SB_DEPLOYER_PORT")
	if port == "" {
		port = "9876"
	}

	ansibleDir := os.Getenv("SB_ANSIBLE_DIR")
	if ansibleDir == "" {
		ansibleDir = "ansible"
	}

	// Prevent path traversal
	ansibleDir = filepath.Clean(ansibleDir)
	if filepath.IsAbs(ansibleDir) {
		log.Fatalf("SB_ANSIBLE_DIR must be a relative path, got: %s", ansibleDir)
	}

	authToken := os.Getenv("SB_AUTH_TOKEN")
	if authToken == "" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("Failed to generate auth token: %v", err)
		}
		authToken = hex.EncodeToString(b)
	}

	return &Config{
		Port:       port,
		AnsibleDir: ansibleDir,
		AuthToken:  authToken,
		TLSCert:    os.Getenv("SB_TLS_CERT"),
		TLSKey:     os.Getenv("SB_TLS_KEY"),
	}
}
