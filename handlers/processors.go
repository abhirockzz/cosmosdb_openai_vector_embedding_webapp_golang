package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/textsplitter"
)

func createCosmosClient(endpoint string) (*azcosmos.Client, error) {
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		// fmt.Println("failed to create cosmosdb client using Azure credential:", err)
		return nil, fmt.Errorf("failed to create Azure credential: %v", err)
	}

	return azcosmos.NewClient(endpoint, credential, nil)
}

func createOpenAIClient(endpoint string) (*azopenai.Client, error) {
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		// fmt.Println("failed to create openai client using Azure credential:", err)
		return nil, fmt.Errorf("failed to create Azure credential: %v", err)
	}

	return azopenai.NewClient(endpoint, credential, nil)
}

func processURL(url string, container *azcosmos.ContainerClient, openAIClient *azopenai.Client, config Config, handler *Handler) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read URL content: %w", err)
	}

	return processContent(bytes.NewReader(content), url, container, openAIClient, config, handler)
}

func processLocalFiles(files []File, container *azcosmos.ContainerClient, openAIClient *azopenai.Client, config Config, handler *Handler) error {
	for i, file := range files {
		// Update progress status for current file
		handler.updateProgress(len(files), i, fmt.Sprintf("Processing file %d of %d: %s", i+1, len(files), file.Name), "")

		// Get file content from memory
		content, err := io.ReadAll(file.Reader)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", file.Name, err)
		}
		defer file.Reader.Close()

		if err := processContent(bytes.NewReader(content), file.Name, container, openAIClient, config, handler); err != nil {
			return fmt.Errorf("failed to process file %s: %w", file.Name, err)
		}
	}

	// Update final progress
	handler.updateProgress(len(files), len(files), "Completed", "")
	return nil
}

func processContent(content io.Reader, source string, container *azcosmos.ContainerClient, openAIClient *azopenai.Client, config Config, handler *Handler) error {
	var splitter textsplitter.TextSplitter
	if filepath.Ext(source) == ".md" {
		splitter = textsplitter.NewMarkdownTextSplitter(
			textsplitter.WithChunkSize(config.ChunkSize),
			textsplitter.WithChunkOverlap(config.ChunkOverlap),
		)
	} else {
		splitter = textsplitter.NewRecursiveCharacter(
			textsplitter.WithChunkSize(config.ChunkSize),
			textsplitter.WithChunkOverlap(config.ChunkOverlap),
		)
	}

	var loader documentloaders.Loader
	if filepath.Ext(source) == ".pdf" {
		buf, err := io.ReadAll(content)
		if err != nil {
			return fmt.Errorf("failed to read PDF content: %w", err)
		}
		loader = documentloaders.NewPDF(bytes.NewReader(buf), int64(len(buf)))
	} else if source[len(source)-5:] == ".html" {
		log.Println("source is a html file, using html loader")
		loader = documentloaders.NewHTML(content)
	} else if source[len(source)-4:] == ".csv" {
		log.Println("source is a csv file, using csv loader")
		loader = documentloaders.NewCSV(content)
	} else {
		log.Println("using text loader")
		loader = documentloaders.NewText(content)
	}

	docs, err := loader.LoadAndSplit(context.Background(), splitter)
	if err != nil {
		return fmt.Errorf("failed to load and split content: %w", err)
	}

	totalDocs := len(docs)
	for i, doc := range docs {
		// Update progress for each document chunk
		handler.updateProgress(totalDocs, i, fmt.Sprintf("Processing chunk %d of %d from %s", i+1, totalDocs, source), "")

		resp, err := openAIClient.GetEmbeddings(context.Background(), azopenai.EmbeddingsOptions{
			Input:          []string{doc.PageContent},
			DeploymentName: &config.EmbeddingModel,
		}, nil)

		if err != nil {
			fmt.Println("failed to generate embedding:", err)
			return fmt.Errorf("failed to generate embedding: %w", err)
		}

		embedding := resp.Data[0].Embedding

		id := uuid.New().String()
		item := map[string]any{
			"id":                      id,
			config.TextAttribute:      doc.PageContent,
			config.EmbeddingAttribute: embedding,
			config.MetadataAttribute: map[string]string{
				"source": source,
			},
		}

		itemBytes, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal item: %w", err)
		}

		_, err = container.CreateItem(context.Background(), azcosmos.NewPartitionKeyString(id), itemBytes, nil)
		if err != nil {
			fmt.Println("failed to create item:", err)
			return fmt.Errorf("failed to create item: %w", err)
		}
	}

	return nil
}
