// Copyright (c) Microsoft. All rights reserved.

package main

import (
	"cmp"
	"context"
	"fmt"
	"os"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/geminiprovider"
	"github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/functool"
	"google.golang.org/genai"
)

var (
	apiKey = os.Getenv("GEMINI_API_KEY")
	model  = cmp.Or(os.Getenv("GEMINI_MODEL"), "gemini-2.5-flash")
)

var logger = demo.NewLogger(
	"Function Tools",
	"Demonstrates a Gemini agent calling a function tool.",
	"Model", model,
)

var weatherTool = functool.MustNew(functool.Config{
	Name:        "weather",
	Description: "Get the current weather for a given location",
}, func(_ context.Context, location string) (string, error) {
	return fmt.Sprintf("The weather in %s is cloudy with a high of 15°C.", location), nil
})

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		demo.Panicf("failed to create Gemini client: %v", err)
	}

	// Create a Gemini agent with a function tool. The provider wires the
	// automatic tool-calling loop, so the model's tool call is executed and its
	// result fed back automatically.
	a := geminiprovider.NewAgent(
		client,
		geminiprovider.AgentConfig{
			Model:        model,
			Instructions: "You are a helpful assistant. Use the provided tools to answer questions.",
			Config: agent.Config{
				Name:        "Weather Assistant",
				Middlewares: []agent.Middleware{logger}, // for logging agent interactions
				Tools:       []tool.Tool{weatherTool},
			},
		},
	)

	// Non-streaming interaction with function tools.
	resp, err := a.RunText(ctx, "What is the weather like in Amsterdam?").Collect()
	demo.Response(resp, err)

	// Streaming interaction with function tools.
	for update, err := range a.RunText(ctx, "What is the weather like in Amsterdam?", agent.Stream(true)) {
		demo.Response(update, err)
	}
}
