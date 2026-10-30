// Copyright (c) Microsoft. All rights reserved.

// This example backs an agent with Azure OpenAI Chat Completions using API-key
// authentication, rather than the Azure AD token credential shown in the
// sibling openai_chat_completion example. Set AZURE_OPENAI_ENDPOINT and
// AZURE_OPENAI_API_KEY (and optionally AZURE_OPENAI_DEPLOYMENT_NAME).
package main

import (
	"context"
	"os"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/openaiprovider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
)

var apiKey = strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_KEY"))

var logger = demo.NewLogger(
	"Azure OpenAI Chat Completions (API key)",
	"Demonstrates an Azure OpenAI chat-completions agent authenticated with an API key.",
	"Model", demo.Deployment,
)

func main() {
	if apiKey == "" {
		demo.Panic("AZURE_OPENAI_API_KEY environment variable is not set.")
	}

	a := openaiprovider.NewChatCompletionsAgent(
		openai.NewClient(
			azure.WithEndpoint(demo.Endpoint, "2024-10-21"),
			azure.WithAPIKey(apiKey),
		),
		openaiprovider.AgentConfig{
			Model:        demo.Deployment,
			Instructions: "You are good at telling jokes.",
			Config: agent.Config{
				Name:        "Joker",
				Middlewares: []agent.Middleware{logger},
			},
		},
	)

	resp, err := a.RunText(context.Background(), "Tell me a joke about a pirate.").Collect()
	demo.Response(resp, err)
}
