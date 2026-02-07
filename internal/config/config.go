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
	ScriptPath string
	AuthToken  string
	TLSCert    string
	TLSKey     string
}

func Load() *Config {
	port := os.Getenv("SB_DEPLOYER_PORT")
	if port == "" {
		port = "8080"
	}

	scriptPath := os.Getenv("SB_SCRIPT_PATH")
	if scriptPath == "" {
		scriptPath = "scripts/install-stackbill-poc.sh"
	}

	// Prevent path traversal in script path
	scriptPath = filepath.Clean(scriptPath)
	if filepath.IsAbs(scriptPath) {
		log.Fatalf("SB_SCRIPT_PATH must be a relative path, got: %s", scriptPath)
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
		ScriptPath: scriptPath,
		AuthToken:  authToken,
		TLSCert:    os.Getenv("SB_TLS_CERT"),
		TLSKey:     os.Getenv("SB_TLS_KEY"),
	}
}
