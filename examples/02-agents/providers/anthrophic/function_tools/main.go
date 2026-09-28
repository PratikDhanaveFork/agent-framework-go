// Copyright (c) Microsoft. All rights reserved.

package main

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/anthropicprovider"
	"github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/functool"
)

var logger = demo.NewLogger(
	"Function Tools",
	"Demonstrates an Anthropic agent calling a function tool.",
	"Model", "claude-sonnet-4-5",
)

var weatherTool = functool.MustNew(functool.Config{
	Name:        "weather",
	Description: "Get the current weather for a given location",
}, func(_ context.Context, location string) (string, error) {
	return fmt.Sprintf("The weather in %s is cloudy with a high of 15°C.", location), nil
})

func main() {
	// Create an Anthropic agent with a function tool. The provider wires the
	// automatic tool-calling loop, so the model's tool call is executed and its
	// result fed back automatically.
	a := anthropicprovider.NewAgent(
		anthropic.NewClient(),
		anthropicprovider.AgentConfig{
			Model:        "claude-sonnet-4-5",
			Instructions: "You are a helpful assistant. Use the provided tools to answer questions.",
			Config: agent.Config{
				Name:        "Weather Assistant",
				Middlewares: []agent.Middleware{logger}, // for logging agent interactions
				Tools:       []tool.Tool{weatherTool},
			},
		},
	)

	ctx := context.Background()

	// Non-streaming interaction with function tools.
	resp, err := a.RunText(ctx, "What is the weather like in Amsterdam?").Collect()
	demo.Response(resp, err)

	// Streaming interaction with function tools.
	for update, err := range a.RunText(ctx, "What is the weather like in Amsterdam?", agent.Stream(true)) {
		demo.Response(update, err)
	}
}
