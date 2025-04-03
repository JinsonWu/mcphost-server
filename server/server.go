package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"mcphost-server/pkg/history"

	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

// Server represents our HTTP server
type Server struct {
	port int
}

// NewServer creates a new server instance
func NewServer() *Server {
	port, err := strconv.Atoi(os.Getenv("MCP_SERVER_PORT"))
	if err != nil {
		log.Error("Failed to parse port", "error", err)
		port = 8115
	}
	return &Server{
		port: port,
	}
}

func LoadServerEnv() {
	if err := godotenv.Load(".env"); err != nil {
		log.Error("Failed to load environment variables", "error", err)
	}

	configFile = os.Getenv("MCP_CONFIG_PATH")
	messageWindow, _ = strconv.Atoi(os.Getenv("MCP_MESSAGE_WINDOW"))
	modelFlag = os.Getenv("MCP_MODEL")
	anthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	anthropicBaseURL = os.Getenv("ANTHROPIC_BASE_URL")
	openaiBaseURL = os.Getenv("OPENAI_BASE_URL")
	openaiAPIKey = os.Getenv("OPENAI_API_KEY")
	log.Info("Environment variables loaded")
}

// healthHandler handles health check requests
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Server is healthy")
}

func (s *Server) promptHandler(w http.ResponseWriter, r *http.Request) {
	messages := make([]history.HistoryMessage, 0)
	prompt := r.FormValue("prompt")
	if prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	if err := runPrompt(prompt, &messages); err != nil {
		http.Error(w, fmt.Sprintf("Error executing prompt: %v", err), http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(messages)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshaling response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func (s *Server) toolHandler(w http.ResponseWriter, r *http.Request) {
	jsonData, err := json.Marshal(modelTools)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshaling response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func (s *Server) historyHandler(w http.ResponseWriter, r *http.Request) {
	var returnMessages []history.HistoryMessage
	log.Info("History requested", "messageWindow", messageWindow, "modelMessages", len(modelMessages))
	if len(modelMessages) > messageWindow {
		returnMessages = modelMessages[len(modelMessages)-messageWindow:]
	} else {
		returnMessages = modelMessages
	}

	jsonData, err := json.Marshal(returnMessages)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshaling response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/tool", s.toolHandler)
	mux.HandleFunc("/history", s.historyHandler)
	mux.HandleFunc("/prompt", s.promptHandler)

	runMCPHost()

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	log.Info("Server hosting", "port", s.port)
	return server.ListenAndServe()
}
