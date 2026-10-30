// Copyright (c) Microsoft. All rights reserved.

// This sample resolves a remote agent's A2A agent card, prints the skills the
// agent advertises, and builds a local agent that delegates to it. Agent cards
// describe an agent's capabilities as a list of skills (id, name, description,
// examples); a client can inspect them before deciding how to use the agent.
// Set A2A_AGENT_URL to the base URL of a running A2A agent.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/a2aprovider"
)

func main() {
	baseURL := strings.TrimSpace(os.Getenv("A2A_AGENT_URL"))
	if baseURL == "" {
		demo.Panic("A2A_AGENT_URL environment variable is not set.")
	}

	logger := demo.NewLogger(
		"A2A Agent Card Skills",
		"Demonstrates resolving a remote agent's card and inspecting its advertised skills.",
	)

	ctx := context.Background()

	// Resolve the public agent card published at the agent's base URL.
	resolver := agentcard.NewResolver(http.DefaultClient)
	card, err := resolver.Resolve(ctx, baseURL)
	if err != nil {
		demo.Panic(err)
	}

	// Inspect the advertised skills before using the agent.
	fmt.Printf("Agent %q advertises %d skill(s):\n", card.Name, len(card.Skills))
	for _, skill := range card.Skills {
		fmt.Printf("  - %s (%s): %s\n", skill.Name, skill.ID, skill.Description)
	}

	// Build a local agent from the resolved card and delegate a request to it.
	a, err := a2aprovider.NewAgentFromCard(ctx, card, a2aprovider.AgentConfig{
		Config: agent.Config{
			Middlewares: []agent.Middleware{logger}, // for logging agent interactions
		},
	})
	if err != nil {
		demo.Panic(err)
	}

	resp, err := a.RunText(ctx, "Briefly, what can you help me with?").Collect()
	demo.Response(resp, err)
}
