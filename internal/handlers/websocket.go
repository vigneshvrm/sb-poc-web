package handlers

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

type WebSocketHandler struct {
	clients map[string]map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewWebSocketHandler() *WebSocketHandler {
	return &WebSocketHandler{
		clients: make(map[string]map[*websocket.Conn]bool),
	}
}

func (h *WebSocketHandler) HandleConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deployID := vars["id"]

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	h.mu.Lock()
	if h.clients[deployID] == nil {
		h.clients[deployID] = make(map[*websocket.Conn]bool)
	}
	h.clients[deployID][conn] = true
	h.mu.Unlock()

	log.Printf("WebSocket client connected for deployment %s", deployID)

	// Keep connection alive, remove on close
	defer func() {
		h.mu.Lock()
		delete(h.clients[deployID], conn)
		if len(h.clients[deployID]) == 0 {
			delete(h.clients, deployID)
		}
		h.mu.Unlock()
		conn.Close()
	}()

	// Read loop (handles pings/close)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *WebSocketHandler) Broadcast(deployID, message string) {
	h.mu.RLock()
	clients := h.clients[deployID]
	h.mu.RUnlock()

	for conn := range clients {
		if err := conn.WriteJSON(map[string]string{
			"deployment_id": deployID,
			"log":           message,
		}); err != nil {
			log.Printf("WebSocket write error: %v", err)
			conn.Close()
			h.mu.Lock()
			delete(h.clients[deployID], conn)
			h.mu.Unlock()
		}
	}
}
