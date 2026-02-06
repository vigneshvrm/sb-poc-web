package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/deployer"
	"stackbill-deployer/internal/models"

	"github.com/gorilla/mux"
)

// SSEEvent represents a server-sent event with a type and data payload.
type SSEEvent struct {
	Type string // "log", "stage", or "done"
	Data string // JSON or plain text payload
}

type APIHandler struct {
	cfg         *config.Config
	deployer    *deployer.Deployer
	deployments map[string]*models.Deployment
	mu          sync.RWMutex
	subscribers map[string][]chan SSEEvent
	subMu       sync.Mutex
}

func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{
		cfg:         cfg,
		deployer:    deployer.New(cfg),
		deployments: make(map[string]*models.Deployment),
		subscribers: make(map[string][]chan SSEEvent),
	}
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
	if req.SSLMode != "letsencrypt" && req.SSLMode != "custom" {
		http.Error(w, `{"error": "ssl_mode must be 'letsencrypt' or 'custom'"}`, http.StatusBadRequest)
		return
	}
	if req.SSLMode == "letsencrypt" && req.LetsEncryptEmail == "" {
		http.Error(w, `{"error": "letsencrypt_email is required when ssl_mode is 'letsencrypt'"}`, http.StatusBadRequest)
		return
	}
	if req.SSLMode == "custom" && (req.SSLCert == "" || req.SSLKey == "") {
		http.Error(w, `{"error": "ssl_cert and ssl_key are required when ssl_mode is 'custom'"}`, http.StatusBadRequest)
		return
	}
	if req.CloudStackMode != "existing" && req.CloudStackMode != "simulator" {
		http.Error(w, `{"error": "cloudstack_mode must be 'existing' or 'simulator'"}`, http.StatusBadRequest)
		return
	}
	if req.ECRToken == "" {
		http.Error(w, `{"error": "ecr_token is required"}`, http.StatusBadRequest)
		return
	}

	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	id := generateID()
	stages := models.BuildStages(req)
	dep := &models.Deployment{
		ID:           id,
		Request:      req,
		Status:       models.StatusPending,
		StartedAt:    time.Now(),
		Logs:         []string{},
		Stages:       stages,
		CurrentStage: -1,
	}

	h.mu.Lock()
	h.deployments[id] = dep
	h.mu.Unlock()

	go h.runDeployment(dep)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     id,
		"status": "pending",
		"stages": stages,
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

		// Broadcast log line via SSE
		h.broadcast(dep.ID, SSEEvent{Type: "log", Data: line})

		// Detect stage transitions from log_step output
		h.detectStage(dep, line)

		log.Printf("[%s] %s", dep.ID, line)
	})

	h.mu.Lock()
	now := time.Now()
	dep.EndedAt = &now
	if err != nil {
		dep.Status = models.StatusFailed
		errMsg := "ERROR: " + err.Error()
		dep.Logs = append(dep.Logs, errMsg)
		// Mark current running stage as error
		if dep.CurrentStage >= 0 && dep.CurrentStage < len(dep.Stages) {
			dep.Stages[dep.CurrentStage].Status = "error"
		}
		h.mu.Unlock()
		h.broadcast(dep.ID, SSEEvent{Type: "log", Data: errMsg})
	} else {
		dep.Status = models.StatusSuccess
		// Mark all remaining stages as done
		for i := range dep.Stages {
			if dep.Stages[i].Status == "running" || dep.Stages[i].Status == "pending" {
				dep.Stages[i].Status = "done"
			}
		}
		h.mu.Unlock()
	}

	doneData, _ := json.Marshal(map[string]interface{}{
		"status": dep.Status,
		"stages": dep.Stages,
	})
	h.broadcast(dep.ID, SSEEvent{Type: "done", Data: string(doneData)})

	// Close all subscriber channels
	h.subMu.Lock()
	if subs, ok := h.subscribers[dep.ID]; ok {
		for _, ch := range subs {
			close(ch)
		}
		delete(h.subscribers, dep.ID)
	}
	h.subMu.Unlock()
}

// detectStage checks if a log line matches a known stage name and updates stage status.
func (h *APIHandler) detectStage(dep *models.Deployment, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i, stage := range dep.Stages {
		if stage.Status != "pending" {
			continue
		}
		// Match against the stage name appearing in log_step output
		if strings.Contains(line, stage.Name) {
			// Mark previous running stage as done
			if dep.CurrentStage >= 0 && dep.CurrentStage < len(dep.Stages) {
				dep.Stages[dep.CurrentStage].Status = "done"
			}
			dep.Stages[i].Status = "running"
			dep.CurrentStage = i

			doneCount := 0
			for _, s := range dep.Stages {
				if s.Status == "done" {
					doneCount++
				}
			}

			stageData, _ := json.Marshal(map[string]interface{}{
				"index":      i,
				"name":       stage.Name,
				"status":     "running",
				"done_count": doneCount,
				"total":      len(dep.Stages),
			})
			// Broadcast stage event (unlock not needed since broadcast uses subMu)
			go h.broadcast(dep.ID, SSEEvent{Type: "stage", Data: string(stageData)})
			break
		}
	}
}

// subscribe creates a channel for an SSE client to receive events.
func (h *APIHandler) subscribe(deployID string) chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	h.subMu.Lock()
	h.subscribers[deployID] = append(h.subscribers[deployID], ch)
	h.subMu.Unlock()
	return ch
}

// unsubscribe removes a channel from the subscriber list.
func (h *APIHandler) unsubscribe(deployID string, ch chan SSEEvent) {
	h.subMu.Lock()
	defer h.subMu.Unlock()
	subs := h.subscribers[deployID]
	for i, s := range subs {
		if s == ch {
			h.subscribers[deployID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// broadcast sends an SSE event to all subscribers for a deployment.
func (h *APIHandler) broadcast(deployID string, event SSEEvent) {
	h.subMu.Lock()
	subs := make([]chan SSEEvent, len(h.subscribers[deployID]))
	copy(subs, h.subscribers[deployID])
	h.subMu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop event if channel is full
		}
	}
}

// StreamSSE handles the SSE endpoint for real-time deployment streaming.
func (h *APIHandler) StreamSSE(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.mu.RLock()
	dep, ok := h.deployments[id]
	h.mu.RUnlock()

	if !ok {
		http.Error(w, `{"error": "deployment not found"}`, http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send catch-up: existing stages
	h.mu.RLock()
	stagesJSON, _ := json.Marshal(dep.Stages)
	h.mu.RUnlock()
	fmt.Fprintf(w, "event: stages\ndata: %s\n\n", stagesJSON)
	flusher.Flush()

	// Send catch-up: existing logs
	h.mu.RLock()
	existingLogs := make([]string, len(dep.Logs))
	copy(existingLogs, dep.Logs)
	currentStatus := dep.Status
	h.mu.RUnlock()

	for _, line := range existingLogs {
		fmt.Fprintf(w, "event: log\ndata: %s\n\n", line)
	}
	flusher.Flush()

	// If already finished, send done event and return
	if currentStatus == models.StatusSuccess || currentStatus == models.StatusFailed {
		h.mu.RLock()
		doneData, _ := json.Marshal(map[string]interface{}{
			"status": dep.Status,
			"stages": dep.Stages,
		})
		h.mu.RUnlock()
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
		flusher.Flush()
		return
	}

	// Subscribe to live events
	ch := h.subscribe(id)
	defer h.unsubscribe(id, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		}
	}
}

func (h *APIHandler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	deps := make([]*models.Deployment, 0, len(h.deployments))
	for _, d := range h.deployments {
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
