package server

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"runtime"

	"stackbill-deployer/internal/config"
	"stackbill-deployer/internal/handlers"

	"github.com/gorilla/mux"
)

func getProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func Run() {
	cfg := config.Load()
	root := getProjectRoot()

	// Parse templates
	tmplPath := filepath.Join(root, "web", "templates", "*.html")
	tmpl, err := template.ParseGlob(tmplPath)
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Create handlers
	apiHandler := handlers.NewAPIHandler(cfg)
	wsHandler := handlers.NewWebSocketHandler()
	apiHandler.SetWSHandler(wsHandler)

	// Router
	r := mux.NewRouter()

	// Static files
	staticDir := filepath.Join(root, "web", "static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Page routes
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("GET")

	// API routes
	r.HandleFunc("/api/deploy", apiHandler.Deploy).Methods("POST")
	r.HandleFunc("/api/deployments", apiHandler.ListDeployments).Methods("GET")
	r.HandleFunc("/api/deployments/{id}", apiHandler.GetDeployment).Methods("GET")

	// WebSocket
	r.HandleFunc("/ws/logs/{id}", wsHandler.HandleConnection)

	addr := ":" + cfg.Port
	log.Printf("StackBill Deployer running at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
