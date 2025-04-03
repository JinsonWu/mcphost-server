package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"mcphost-server/pkg/history"
	"mcphost-server/pkg/llm"
	"mcphost-server/pkg/llm/anthropic"
	"mcphost-server/pkg/llm/ollama"
	"mcphost-server/pkg/llm/openai"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	configFile       string
	messageWindow    int = 10
	modelFlag        string
	anthropicAPIKey  string
	anthropicBaseURL string
	openaiBaseURL    string
	openaiAPIKey     string
	modelProvider    llm.Provider
	modelMcpClients  map[string]*mcpclient.StdioMCPClient
	modelTools       []llm.Tool
	modelMessages    []history.HistoryMessage
)

var debugMode bool = false

// Add new function to create provider
func createProvider(modelString string) (llm.Provider, error) {
	parts := strings.SplitN(modelString, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf(
			"invalid model format. Expected provider:model, got %s",
			modelString,
		)
	}

	provider := parts[0]
	model := parts[1]

	switch provider {
	case "anthropic":
		apiKey := anthropicAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}

		if apiKey == "" {
			return nil, fmt.Errorf(
				"anthropic API key not provided. Use --anthropic-api-key flag or ANTHROPIC_API_KEY environment variable",
			)
		}
		return anthropic.NewProvider(apiKey, anthropicBaseURL, model), nil

	case "ollama":
		return ollama.NewProvider(model)

	case "openai":
		apiKey := openaiAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}

		if apiKey == "" {
			return nil, fmt.Errorf(
				"OpenAI API key not provided. Use --openai-api-key flag or OPENAI_API_KEY environment variable",
			)
		}
		return openai.NewProvider(apiKey, openaiBaseURL, model), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// Method implementations for simpleMessage
func runPrompt(
	prompt string,
	messages *[]history.HistoryMessage,
) error {
	var message llm.Message
	var err error

	if prompt != "" {
		log.Info("User prompt", "User", prompt)
		*messages = append(
			*messages,
			history.HistoryMessage{
				Role: "user",
				Content: []history.ContentBlock{{
					Type: "text",
					Text: prompt,
				}},
			},
		)
	}

	llmMessages := make([]llm.Message, len(*messages))
	for i := range *messages {
		llmMessages[i] = &(*messages)[i]
	}

	// Ensure modelProvider is initialized before using it
	if modelProvider == nil {
		log.Error("Model provider not initialized")
		return fmt.Errorf("model provider not initialized")
	}

	message, err = modelProvider.CreateMessage(
		context.Background(),
		prompt,
		llmMessages,
		modelTools,
	)
	if err != nil {
		return err
	}

	var messageContent []history.ContentBlock
	var toolResults []history.ContentBlock

	// Add text content
	if message.GetContent() != "" {
		messageContent = append(messageContent, history.ContentBlock{
			Type: "text",
			Text: message.GetContent(),
		})
		log.Info("Assistant response", "Assistant", message.GetContent())
	}

	// Handle tool calls
	for _, toolCall := range message.GetToolCalls() {
		log.Info("🔧 Using tool", "name", toolCall.GetName())

		input, _ := json.Marshal(toolCall.GetArguments())
		messageContent = append(messageContent, history.ContentBlock{
			Type:  "tool_use",
			ID:    toolCall.GetID(),
			Name:  toolCall.GetName(),
			Input: input,
		})

		// Log usage statistics if available
		inputTokens, outputTokens := message.GetUsage()
		if inputTokens > 0 || outputTokens > 0 {
			log.Info("Usage statistics",
				"input_tokens", inputTokens,
				"output_tokens", outputTokens,
				"total_tokens", inputTokens+outputTokens)
		}

		parts := strings.Split(toolCall.GetName(), "__")
		if len(parts) != 2 {
			log.Warn(
				"Invalid tool name format",
				"name", toolCall.GetName(),
			)
			continue
		}

		serverName, toolName := parts[0], parts[1]
		mcpClient, ok := modelMcpClients[serverName]
		if !ok {
			log.Warn("Server not found", "server", serverName)
			continue
		}

		var toolArgs map[string]interface{}
		if err := json.Unmarshal(input, &toolArgs); err != nil {
			log.Warn("Error parsing tool arguments", "error", err)
			continue
		}

		var toolResultPtr *mcp.CallToolResult

		req := mcp.CallToolRequest{}
		req.Params.Name = toolName
		req.Params.Arguments = toolArgs
		toolResultPtr, err = mcpClient.CallTool(
			context.Background(),
			req,
		)

		if err != nil {
			errMsg := fmt.Sprintf(
				"Error calling tool %s: %v",
				toolName,
				err,
			)
			log.Warn("Error calling tool", "error", errMsg)

			// Add error message as tool result
			messageContent = append(messageContent, history.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolCall.GetID(),
				Content: []history.ContentBlock{{
					Type: "text",
					Text: errMsg,
				}},
			})
			continue
		}

		toolResult := *toolResultPtr

		if toolResult.Content != nil {
			log.Info("Raw tool result content", "content", toolResult.Content)

			// Create the tool result block
			resultBlock := history.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolCall.GetID(),
				Content:   toolResult.Content,
			}

			// Extract text content
			var resultText string
			// Handle array content directly since we know it's []interface{}
			for _, item := range toolResult.Content {
				if contentMap, ok := item.(map[string]interface{}); ok {
					if text, ok := contentMap["text"]; ok {
						resultText += fmt.Sprintf("%v ", text)
					}
				}
			}

			resultBlock.Text = strings.TrimSpace(resultText)
			log.Info("created tool result block",
				"block", resultBlock,
				"tool_id", toolCall.GetID())

			toolResults = append(toolResults, resultBlock)
		}
	}

	*messages = append(*messages, history.HistoryMessage{
		Role:    message.GetRole(),
		Content: messageContent,
	})

	if len(toolResults) > 0 {
		*messages = append(*messages, history.HistoryMessage{
			Role:    "user",
			Content: toolResults,
		})
		// Make another call to get Claude's response to the tool results
		return runPrompt("", messages)
	}

	modelMessages = append(modelMessages, *messages...)
	return nil
}

func runMCPHost() error {
	// Set up logging based on debug flag
	if debugMode {
		log.SetLevel(log.DebugLevel)
		// Enable caller information for debug logs
		log.SetReportCaller(true)
	} else {
		log.SetLevel(log.InfoLevel)
		log.SetReportCaller(false)
	}

	// Create the provider based on the model flag
	var err error
	modelProvider, err = createProvider(modelFlag)
	if err != nil {
		return fmt.Errorf("error creating provider: %v", err)
	}

	// Split the model flag and get just the model name
	parts := strings.SplitN(modelFlag, ":", 2)
	log.Info("Model loaded",
		"provider", modelProvider.Name(),
		"model", parts[1])

	mcpConfig, err := loadMCPConfig()
	if err != nil {
		log.Error("Error loading MCP config", "error", err)
	}

	modelMcpClients, err = createMCPClients(mcpConfig)
	if err != nil {
		log.Error("Error creating MCP clients", "error", err)
	}

	log.Info("MCP clients created", "count", len(modelMcpClients))
	for name := range modelMcpClients {
		log.Info("Server connected", "name", name)
	}

	for serverName, mcpClient := range modelMcpClients {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		cancel()

		if err != nil {
			log.Error(
				"Error fetching tools",
				"server",
				serverName,
				"error",
				err,
			)
			continue
		}

		serverTools := mcpToolsToAnthropicTools(serverName, toolsResult.Tools)
		modelTools = append(modelTools, serverTools...)
		log.Info(
			"Tools loaded",
			"server",
			serverName,
			"count",
			len(toolsResult.Tools),
		)
	}

	return nil
}
