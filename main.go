package main

import (
	"log"
	"net/http"

	"github.com/abhirockzz/cosmosdb_openai_embeddings_go/handlers"
)

const PORT = ":8080"

func main() {
	// Create handler instance
	handler := handlers.NewHandler()

	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("static")))

	// API endpoints
	http.HandleFunc("/api/config", handler.HandleConfig)
	http.HandleFunc("/api/process", handler.HandleProcess)
	http.HandleFunc("/api/progress", handler.HandleProgress)

	log.Println("Server starting on " + PORT)
	log.Fatal(http.ListenAndServe(PORT, nil))
}
