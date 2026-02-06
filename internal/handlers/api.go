package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/deployer"
	"stackbill-deployer/internal/models"

	"github.com/gorilla/mux"
)

type APIHandler struct {
	cfg         *config.Config
	deployer    *deployer.Deployer
	deployments map[string]*models.Deployment
	mu          sync.RWMutex
	wsHandler   *WebSocketHandler
}

func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{
		cfg:         cfg,
		deployer:    deployer.New(cfg),
		deployments: make(map[string]*models.Deployment),
	}
}

func (h *APIHandler) SetWSHandler(ws *WebSocketHandler) {
	h.wsHandler = ws
}

func (h *APIHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	var req models.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ServerIP == "" {
		http.Error(w, `{"error": "server_ip is required"}`, http.StatusBadRequest)
		return
	}
	if req.SSHUser == "" {
		http.Error(w, `{"error": "ssh_user is required"}`, http.StatusBadRequest)
		return
	}
	if req.SSHPass == "" && req.SSHKeyPath == "" {
		http.Error(w, `{"error": "ssh_pass or ssh_key_path is required"}`, http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		http.Error(w, `{"error": "domain is required"}`, http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	// Create deployment record
	id := generateID()
	dep := &models.Deployment{
		ID:        id,
		Request:   req,
		Status:    models.StatusPending,
		StartedAt: time.Now(),
		Logs:      []string{},
	}

	h.mu.Lock()
	h.deployments[id] = dep
	h.mu.Unlock()

	// Start deployment in background
	go h.runDeployment(dep)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"status": "pending",
	})
}

func (h *APIHandler) runDeployment(dep *models.Deployment) {
	h.mu.Lock()
	dep.Status = models.StatusRunning
	h.mu.Unlock()

	err := h.deployer.Deploy(dep.Request, func(line string) {
		h.mu.Lock()
		dep.Logs = append(dep.Logs, line)
		h.mu.Unlock()

		// Broadcast to WebSocket clients
		if h.wsHandler != nil {
			h.wsHandler.Broadcast(dep.ID, line)
		}

		log.Printf("[%s] %s", dep.ID, line)
	})

	h.mu.Lock()
	now := time.Now()
	dep.EndedAt = &now
	if err != nil {
		dep.Status = models.StatusFailed
		dep.Logs = append(dep.Logs, "ERROR: "+err.Error())
		if h.wsHandler != nil {
			h.wsHandler.Broadcast(dep.ID, "ERROR: "+err.Error())
		}
	} else {
		dep.Status = models.StatusSuccess
	}
	h.mu.Unlock()
}

func (h *APIHandler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	deps := make([]*models.Deployment, 0, len(h.deployments))
	for _, d := range h.deployments {
		// Don't include full logs in list view
		summary := *d
		summary.Logs = nil
		deps = append(deps, &summary)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deps)
}

func (h *APIHandler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.mu.RLock()
	dep, ok := h.deployments[id]
	h.mu.RUnlock()

	if !ok {
		http.Error(w, `{"error": "deployment not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dep)
}

func generateID() string {
	return time.Now().Format("20060102-150405")
}
