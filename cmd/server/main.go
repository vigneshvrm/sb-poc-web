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

	// Router
	r := mux.NewRouter()

	// Security headers on all routes
	r.Use(handlers.SecurityHeaders)

	// Static files (no auth — public assets)
	staticDir := filepath.Join(root, "web", "static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Page routes (no auth — the HTML shell is public, API is protected)
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("GET")

	// API routes (auth required via bearer token)
	api := r.PathPrefix("/api").Subrouter()
	api.Use(apiHandler.AuthMiddleware)
	api.HandleFunc("/deploy", apiHandler.Deploy).Methods("POST")
	api.HandleFunc("/deployments", apiHandler.ListDeployments).Methods("GET")
	api.HandleFunc("/deployments/{id}", apiHandler.GetDeployment).Methods("GET")
	api.HandleFunc("/deployments/{id}/stream", apiHandler.StreamSSE).Methods("GET")
	api.HandleFunc("/deployments/{id}/log", apiHandler.DownloadLog).Methods("GET")

	addr := ":" + cfg.Port

	log.Println("=========================================")
	log.Printf("  StackBill Deployer running on port %s", cfg.Port)
	log.Printf("  Auth Token: %s", cfg.AuthToken)
	log.Println("=========================================")

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		log.Println("  TLS: enabled")
		log.Fatal(http.ListenAndServeTLS(addr, cfg.TLSCert, cfg.TLSKey, r))
	} else {
		log.Fatal(http.ListenAndServe(addr, r))
	}
}
