// Copyright (c) Microsoft. All rights reserved.

// This example runs a Gemini agent through Vertex AI rather than the Gemini
// Developer API. The only difference from the basic Gemini example is how the
// genai client is constructed: it uses the Vertex AI backend with a GCP project
// and location and application default credentials. Set GOOGLE_CLOUD_PROJECT
// and GOOGLE_CLOUD_LOCATION (and have ADC configured, e.g. via
// `gcloud auth application-default login`).
package main

import (
	"cmp"
	"context"
	"os"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/geminiprovider"
	"google.golang.org/genai"
)

var (
	project  = os.Getenv("GOOGLE_CLOUD_PROJECT")
	location = cmp.Or(os.Getenv("GOOGLE_CLOUD_LOCATION"), "us-central1")
	model    = cmp.Or(os.Getenv("GEMINI_MODEL"), "gemini-2.5-flash")
)

var logger = demo.NewLogger(
	"Gemini via Vertex AI",
	"Demonstrates a Gemini agent backed by the Vertex AI backend.",
	"Model", model,
)

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  project,
		Location: location,
	})
	if err != nil {
		demo.Panicf("failed to create Vertex AI Gemini client: %v", err)
	}

	a := geminiprovider.NewAgent(
		client,
		geminiprovider.AgentConfig{
			Model:        model,
			Instructions: "You are good at telling jokes.",
			Config: agent.Config{
				Name:        "Joker",
				Middlewares: []agent.Middleware{logger}, // for logging agent interactions
			},
		},
	)

	resp, err := a.RunText(ctx, "Tell me a joke about a pirate.").Collect()
	demo.Response(resp, err)
}
