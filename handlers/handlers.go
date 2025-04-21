package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

type Config struct {
	CosmosDBEndpoint    string `json:"cosmosDBEndpoint"`
	DatabaseName        string `json:"databaseName"`
	ContainerName       string `json:"containerName"`
	AzureOpenAIEndpoint string `json:"azureOpenAIEndpoint"`
	EmbeddingModel      string `json:"embeddingModel"`
	TextAttribute       string `json:"textAttribute"`
	EmbeddingAttribute  string `json:"embeddingAttribute"`
	MetadataAttribute   string `json:"metadataAttribute"`
	ChunkSize           int    `json:"chunkSize"`
	ChunkOverlap        int    `json:"chunkOverlap"`
	URL                 string `json:"url,omitempty"`
	Files               []File `json:"files,omitempty"`
}

type File struct {
	Name   string
	Type   string
	Reader io.ReadCloser
}

type Progress struct {
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type Handler struct {
	progress     Progress
	progressLock sync.Mutex
	cosmosClient *azcosmos.Client
	openAIClient *azopenai.Client
	clientConfig Config
	clientsLock  sync.RWMutex
	container    *azcosmos.ContainerClient // Corrected type for container
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) updateProgress(total, processed int, status, errMsg string) {
	h.progressLock.Lock()
	defer h.progressLock.Unlock()

	h.progress.Total = total
	h.progress.Processed = processed
	h.progress.Status = status
	h.progress.Error = errMsg
}

func (h *Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		log.Printf("Error decoding config: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// log.Printf("Received configuration: %+v", config)

	// Validate required fields
	if config.CosmosDBEndpoint == "" || config.DatabaseName == "" || config.ContainerName == "" ||
		config.AzureOpenAIEndpoint == "" || config.EmbeddingModel == "" {
		http.Error(w, "Missing required configuration fields", http.StatusBadRequest)
		return
	}

	// Create new clients with the updated configuration
	h.clientsLock.Lock()
	defer h.clientsLock.Unlock()

	// Create CosmosDB client
	newCosmosClient, err := createCosmosClient(config.CosmosDBEndpoint)
	if err != nil {
		log.Printf("Error creating CosmosDB client: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create CosmosDB client: %v", err), http.StatusInternalServerError)
		return
	}

	// Create OpenAI client
	newOpenAIClient, err := createOpenAIClient(config.AzureOpenAIEndpoint)
	if err != nil {
		log.Printf("Error creating OpenAI client: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create OpenAI client: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate database connection
	db, err := newCosmosClient.NewDatabase(config.DatabaseName)
	if err != nil {
		log.Printf("Error validating database: %v", err)
		http.Error(w, fmt.Sprintf("Failed to connect to database: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate container and pre-create it
	container, err := db.NewContainer(config.ContainerName)
	if err != nil {
		log.Printf("Error validating container: %v", err)
		http.Error(w, fmt.Sprintf("Failed to connect to container: %v", err), http.StatusInternalServerError)
		return
	}

	// Update cached clients, config, and container
	h.cosmosClient = newCosmosClient
	h.openAIClient = newOpenAIClient
	h.clientConfig = config
	h.container = container

	log.Println("Configuration saved successfully")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (h *Handler) getClients() (*azopenai.Client, *azcosmos.ContainerClient, Config, error) {
	h.clientsLock.RLock()
	defer h.clientsLock.RUnlock()

	if h.openAIClient == nil || h.container == nil {
		return nil, nil, Config{}, fmt.Errorf("clients not initialized")
	}

	return h.openAIClient, h.container, h.clientConfig, nil
}

func (h *Handler) HandleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get cached clients and container
	openAIClient, container, config, err := h.getClients() // Removed azcosmos.Client from return values
	if err != nil {
		http.Error(w, "Please configure the application first", http.StatusPreconditionFailed)
		return
	}

	// Parse multipart form with 10MB max memory
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Reset progress
	h.updateProgress(0, 0, "Starting...", "")

	// Process URL if provided
	url := r.FormValue("url")
	if url != "" {
		if err := processURL(url, container, openAIClient, config, h); err != nil {
			h.updateProgress(0, 0, "Error", fmt.Sprintf("Failed to process URL: %v", err))
			http.Error(w, "Failed to process URL", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("URL processed successfully"))
		return
	}

	// Process files if provided
	files := r.MultipartForm.File["files"]
	if len(files) > 0 {
		h.updateProgress(len(files), 0, "Processing files...", "")

		// Create File structs with readers
		var fileStructs []File
		for _, fileHeader := range files {
			// Open the uploaded file
			file, err := fileHeader.Open()
			if err != nil {
				h.updateProgress(0, 0, "Error", fmt.Sprintf("Failed to open file: %v", err))
				http.Error(w, "Failed to open file", http.StatusInternalServerError)
				return
			}

			fileStructs = append(fileStructs, File{
				Name:   fileHeader.Filename,
				Type:   fileHeader.Header.Get("Content-Type"),
				Reader: file,
			})
		}

		// Process the files
		if err := processLocalFiles(fileStructs, container, openAIClient, config, h); err != nil {
			h.updateProgress(0, 0, "Error", fmt.Sprintf("Failed to process files: %v", err))
			http.Error(w, "Failed to process files", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Files processed successfully"))
		return
	}

	http.Error(w, "No URL or files provided", http.StatusBadRequest)
}

func (h *Handler) HandleProgress(w http.ResponseWriter, r *http.Request) {
	h.progressLock.Lock()
	defer h.progressLock.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.progress)
}
