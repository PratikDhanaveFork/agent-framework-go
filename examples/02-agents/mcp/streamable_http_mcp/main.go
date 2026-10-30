// Copyright (c) Microsoft. All rights reserved.

// This sample connects an agent to a remote MCP server over the streamable
// HTTP transport, as opposed to the local stdio transport shown in
// local_stdio_mcp. It connects to the MCP endpoint, lists the tools the server
// exposes, and attaches them to a Foundry agent. Set MCP_SERVER_URL to the
// server's streamable HTTP endpoint (for example
// "https://example.com/mcp") and FOUNDRY_PROJECT_ENDPOINT for the agent.
package main

import (
	"context"
	"os"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/provider/foundryprovider"
	"github.com/microsoft/agent-framework-go/tool/mcptool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	serverURL := strings.TrimSpace(os.Getenv("MCP_SERVER_URL"))
	if serverURL == "" {
		demo.Panic("MCP_SERVER_URL environment variable is not set.")
	}

	logger := demo.NewLogger(
		"Streamable HTTP MCP Client",
		"Demonstrates connecting an agent to a remote MCP server over streamable HTTP.",
		"Model", demo.FoundryModel,
	)

	ctx := context.Background()
	token := demo.FoundryTokenCredential()

	// Connect to the remote MCP server over the streamable HTTP transport.
	session, err := mcptool.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: serverURL})
	if err != nil {
		demo.Panic(err)
	}
	defer func() { _ = session.Close() }()

	// Discover the tools the server exposes.
	tools, err := mcptool.ListTools(ctx, session)
	if err != nil {
		demo.Panic(err)
	}

	a := foundryprovider.NewAgent(
		demo.FoundryProjectEndpoint,
		token,
		foundryprovider.ModelDeployment(demo.FoundryModel),
		foundryprovider.AgentConfig{
			Instructions: "You are a helpful assistant. Use the available tools to answer the user's request.",
			Config: agent.Config{
				Middlewares: []agent.Middleware{logger}, // for logging agent interactions
				Tools:       tools,
			},
		},
	)

	resp, err := a.RunText(ctx, "Use the available tools to help answer: what can you do?").Collect()
	demo.Response(resp, err)
}
