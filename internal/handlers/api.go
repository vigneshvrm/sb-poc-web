package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/deployer"
	"stackbill-deployer/internal/models"

	"github.com/gorilla/mux"
)

// Validation patterns
var (
	validIDRegex      = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)
	validDomainRegex  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`)
	validVersionRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

// SSEEvent represents a server-sent event with a type and data payload.
type SSEEvent struct {
	Type string // "log", "stage", or "done"
	Data string // JSON or plain text payload
}

type APIHandler struct {
	cfg           *config.Config
	deployer      *deployer.Deployer
	deployments   map[string]*models.Deployment
	mu            sync.RWMutex
	subscribers   map[string][]chan SSEEvent
	subMu         sync.Mutex
	activeServers map[string]bool // Prevent concurrent deploys to same server
	serverMu      sync.Mutex
	lastDeploy    time.Time // Simple rate limiting
}

func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{
		cfg:           cfg,
		deployer:      deployer.New(cfg),
		deployments:   make(map[string]*models.Deployment),
		subscribers:   make(map[string][]chan SSEEvent),
		activeServers: make(map[string]bool),
	}
}

// AuthMiddleware validates the bearer token on API routes.
func (h *APIHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""

		// Check Authorization header first
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}

		// Fallback: query parameter (required for EventSource which can't set headers)
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != h.cfg.AuthToken {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid or missing auth token"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders adds protective headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

func (h *APIHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req models.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// --- Input validation ---

	// Server IP must be a valid IP address
	if net.ParseIP(req.ServerIP) == nil {
		http.Error(w, `{"error": "server_ip must be a valid IP address"}`, http.StatusBadRequest)
		return
	}

	if req.SSHUser == "" {
		http.Error(w, `{"error": "ssh_user is required"}`, http.StatusBadRequest)
		return
	}
	if req.SSHPass == "" {
		http.Error(w, `{"error": "ssh_pass is required"}`, http.StatusBadRequest)
		return
	}

	// Domain format validation
	if req.Domain == "" || !validDomainRegex.MatchString(req.Domain) {
		http.Error(w, `{"error": "domain must be a valid domain name"}`, http.StatusBadRequest)
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
	if req.SSLMode == "letsencrypt" && !strings.Contains(req.LetsEncryptEmail, "@") {
		http.Error(w, `{"error": "invalid email format"}`, http.StatusBadRequest)
		return
	}
	if req.SSLMode == "custom" {
		if req.SSLCert == "" || req.SSLKey == "" {
			http.Error(w, `{"error": "ssl_cert and ssl_key are required when ssl_mode is 'custom'"}`, http.StatusBadRequest)
			return
		}
		if !strings.Contains(req.SSLCert, "BEGIN CERTIFICATE") {
			http.Error(w, `{"error": "ssl_cert must be a valid PEM certificate (missing BEGIN CERTIFICATE)"}`, http.StatusBadRequest)
			return
		}
		if !strings.Contains(req.SSLKey, "BEGIN") || !strings.Contains(req.SSLKey, "PRIVATE KEY") {
			http.Error(w, `{"error": "ssl_key must be a valid PEM private key (missing BEGIN PRIVATE KEY)"}`, http.StatusBadRequest)
			return
		}
	}

	if req.CloudStackMode != "existing" && req.CloudStackMode != "simulator" {
		http.Error(w, `{"error": "cloudstack_mode must be 'existing' or 'simulator'"}`, http.StatusBadRequest)
		return
	}
	if req.CloudStackMode == "simulator" && req.CloudStackVersion != "" {
		if !validVersionRegex.MatchString(req.CloudStackVersion) {
			http.Error(w, `{"error": "invalid cloudstack version format"}`, http.StatusBadRequest)
			return
		}
	}

	if req.ECRToken == "" {
		http.Error(w, `{"error": "ecr_token is required"}`, http.StatusBadRequest)
		return
	}

	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	// --- Rate limiting: max 1 deploy per 10 seconds ---
	h.serverMu.Lock()
	if time.Since(h.lastDeploy) < 10*time.Second {
		h.serverMu.Unlock()
		http.Error(w, `{"error": "please wait before starting another deployment"}`, http.StatusTooManyRequests)
		return
	}

	// --- Concurrent deployment guard: one deploy per server ---
	if h.activeServers[req.ServerIP] {
		h.serverMu.Unlock()
		http.Error(w, `{"error": "a deployment is already running on this server"}`, http.StatusConflict)
		return
	}
	h.activeServers[req.ServerIP] = true
	h.lastDeploy = time.Now()
	h.serverMu.Unlock()

	id := generateID()
	stages := models.BuildStages(req)
	dep := &models.Deployment{
		ID:           id,
		Request:      req,
		Summary:      models.NewSummary(req),
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
	// Release the server lock when deployment finishes
	defer func() {
		h.serverMu.Lock()
		delete(h.activeServers, dep.Request.ServerIP)
		h.serverMu.Unlock()
	}()

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

	// Save deployment log to local file
	h.saveDeploymentLog(dep)

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
		// Match against MatchKey if set, otherwise fall back to Name
		matchKey := stage.MatchKey
		if matchKey == "" {
			matchKey = stage.Name
		}
		if strings.Contains(line, matchKey) {
			// Mark ALL stages before the current one as done (handles skipped stages)
			for j := 0; j < i; j++ {
				if dep.Stages[j].Status == "running" || dep.Stages[j].Status == "pending" {
					dep.Stages[j].Status = "done"
				}
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
			h.broadcast(dep.ID, SSEEvent{Type: "stage", Data: string(stageData)})
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

	if !validIDRegex.MatchString(id) {
		http.Error(w, `{"error": "invalid deployment ID"}`, http.StatusBadRequest)
		return
	}

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

	// Subscribe FIRST to avoid missing events during catch-up
	ch := h.subscribe(id)
	defer h.unsubscribe(id, ch)

	// Send catch-up: existing stages
	h.mu.RLock()
	stagesJSON, _ := json.Marshal(dep.Stages)
	existingLogs := make([]string, len(dep.Logs))
	copy(existingLogs, dep.Logs)
	currentStatus := dep.Status
	h.mu.RUnlock()

	fmt.Fprintf(w, "event: stages\ndata: %s\n\n", stagesJSON)
	flusher.Flush()

	// Send catch-up: existing logs
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

	// Drain any events that arrived during catch-up to avoid duplicates
	draining := true
	for draining {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			draining = false
		}
	}

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

	if !validIDRegex.MatchString(id) {
		http.Error(w, `{"error": "invalid deployment ID"}`, http.StatusBadRequest)
		return
	}

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

// saveDeploymentLog writes all deployment logs to a local file.
func (h *APIHandler) saveDeploymentLog(dep *models.Deployment) {
	os.MkdirAll("logs", 0750)

	h.mu.RLock()
	logs := make([]string, len(dep.Logs))
	copy(logs, dep.Logs)
	h.mu.RUnlock()

	logFile := filepath.Join("logs", fmt.Sprintf("stackbill-deploy-%s.log", dep.ID))
	content := strings.Join(logs, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0600); err != nil {
		log.Printf("Failed to save deployment log: %v", err)
	} else {
		log.Printf("Deployment log saved to %s", logFile)
	}
}

// DownloadLog serves the deployment log file as a download.
func (h *APIHandler) DownloadLog(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Validate ID format to prevent path traversal
	if !validIDRegex.MatchString(id) {
		http.Error(w, `{"error": "invalid deployment ID"}`, http.StatusBadRequest)
		return
	}

	logFile := filepath.Join("logs", fmt.Sprintf("stackbill-deploy-%s.log", id))
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		http.Error(w, `{"error": "log file not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="stackbill-deploy-%s.log"`, id))
	http.ServeFile(w, r, logFile)
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}
