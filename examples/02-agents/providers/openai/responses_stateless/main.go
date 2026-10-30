// Copyright (c) Microsoft. All rights reserved.

// This example runs an OpenAI Responses agent in stateless mode by setting
// DisableStoreOutput. With service-side output storage disabled, responses are
// not retained by the service and response IDs are not saved into the session
// for later continuation; instead the provider includes reasoning encrypted
// content by default so reasoning items can be replayed statelessly across
// turns. This suits deployments that must not rely on server-side state.
package main

import (
	"cmp"
	"context"
	"os"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/openaiprovider"
	"github.com/openai/openai-go/v3"
)

var model = cmp.Or(strings.TrimSpace(os.Getenv("OPENAI_CHAT_MODEL_NAME")), "gpt-5.4-mini")

var logger = demo.NewLogger(
	"OpenAI Responses (stateless)",
	"Demonstrates an OpenAI Responses agent with service-side storage disabled.",
	"Model", model,
)

func main() {
	a := openaiprovider.NewResponsesAgent(
		openai.NewClient(),
		openaiprovider.AgentConfig{
			Model:              model,
			Instructions:       "You are good at telling jokes.",
			DisableStoreOutput: true, // stateless: no server-side output storage
			Config: agent.Config{
				Name:        "Joker",
				Middlewares: []agent.Middleware{logger},
			},
		},
	)

	// The session carries conversation history client-side; because storage is
	// disabled, nothing is persisted service-side between turns.
	ctx := context.Background()
	session, err := a.CreateSession(ctx)
	if err != nil {
		demo.Panic(err)
	}

	resp, err := a.RunText(ctx, "Tell me a joke about a pirate.", agent.WithSession(session)).Collect()
	demo.Response(resp, err)

	resp, err = a.RunText(ctx, "Now tell it again as a haiku.", agent.WithSession(session)).Collect()
	demo.Response(resp, err)
}
