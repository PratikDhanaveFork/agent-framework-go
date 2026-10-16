// Copyright (c) Microsoft. All rights reserved.

// This example runs an agent against a local Ollama server. Ollama exposes an
// OpenAI-compatible Chat Completions API, so the bundled OpenAI provider can
// drive it directly by pointing the OpenAI client's base URL at Ollama's
// /v1 endpoint — no dedicated Ollama provider is needed.
//
// Prerequisites: a running Ollama server (https://ollama.com) with the model
// pulled, e.g. `ollama pull llama3.2`. Override the endpoint and model with
// OLLAMA_BASE_URL and OLLAMA_MODEL.
package main

import (
	"cmp"
	"context"
	"os"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/openaiprovider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

var (
	baseURL = cmp.Or(os.Getenv("OLLAMA_BASE_URL"), "http://localhost:11434/v1")
	model   = cmp.Or(os.Getenv("OLLAMA_MODEL"), "llama3.2")
)

var logger = demo.NewLogger(
	"Agent with Ollama",
	"Demonstrates an agent backed by a local Ollama server via the OpenAI-compatible API.",
	"Model", model,
)

func main() {
	// Ollama ignores the API key but the OpenAI client requires a non-empty one.
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("ollama"),
	)

	a := openaiprovider.NewChatCompletionsAgent(
		client,
		openaiprovider.AgentConfig{
			Model:        model,
			Instructions: "You are good at telling jokes.",
			Config: agent.Config{
				Name:        "Joker",
				Middlewares: []agent.Middleware{logger}, // for logging agent interactions
			},
		},
	)

	resp, err := a.RunText(context.Background(), "Tell me a joke about a pirate.").Collect()
	demo.Response(resp, err)
}
