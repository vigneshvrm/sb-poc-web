package config

import "os"

type Config struct {
	Port       string
	ScriptPath string
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

	return &Config{
		Port:       port,
		ScriptPath: scriptPath,
	}
}
