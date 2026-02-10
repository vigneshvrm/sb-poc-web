package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port       string
	AnsibleDir string
	AuthToken  string
	DataDir    string
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

	dataDir := os.Getenv("SB_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	os.MkdirAll(dataDir, 0750)

	// Token priority: env var > data/token file > generate new and save
	authToken := os.Getenv("SB_AUTH_TOKEN")
	if authToken == "" {
		tokenFile := filepath.Join(dataDir, "token")
		if data, err := os.ReadFile(tokenFile); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			authToken = strings.TrimSpace(string(data))
		}
	}
	if authToken == "" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("Failed to generate auth token: %v", err)
		}
		authToken = hex.EncodeToString(b)
		tokenFile := filepath.Join(dataDir, "token")
		if err := os.WriteFile(tokenFile, []byte(authToken+"\n"), 0600); err != nil {
			log.Printf("Warning: could not persist token to %s: %v", tokenFile, err)
		}
	}

	return &Config{
		Port:       port,
		AnsibleDir: ansibleDir,
		AuthToken:  authToken,
		DataDir:    dataDir,
		TLSCert:    os.Getenv("SB_TLS_CERT"),
		TLSKey:     os.Getenv("SB_TLS_KEY"),
	}
}
