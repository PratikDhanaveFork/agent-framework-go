// Copyright (c) Microsoft. All rights reserved.

// This example shows model/agent routing: a lightweight router inspects each
// request and dispatches it to one of several specialized agents. In a real
// deployment each agent would be backed by a different model (a small, cheap
// model for simple requests and a larger one for hard reasoning, say); here
// both are backed by deterministic in-process providers so the example runs
// without credentials and produces stable output.
package main

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/message"
)

// cannedAgent builds an agent whose provider returns a fixed, labeled reply.
// Stand-ins for real provider-backed agents so routing is easy to observe.
func cannedAgent(name, reply string) *agent.Agent {
	run := func(_ context.Context, _ []*message.Message, _ ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
		return func(yield func(*agent.ResponseUpdate, error) bool) {
			yield(&agent.ResponseUpdate{
				Role:     message.RoleAssistant,
				Contents: message.Contents{&message.TextContent{Text: reply}},
			}, nil)
		}
	}
	return agent.New(agent.ProviderConfig{ProviderName: name, Run: run}, agent.Config{Name: name})
}

func main() {
	// Two specialized agents. Swap the providers for real model-backed ones
	// (e.g. a fast model vs. a reasoning model) without changing the routing.
	codeAgent := cannedAgent("code-model", "[code-model] here is a code review.")
	chatAgent := cannedAgent("chat-model", "[chat-model] happy to chat!")

	// route picks the agent best suited to the request. A production router
	// might use a classifier or the request metadata instead of keywords.
	route := func(prompt string) *agent.Agent {
		p := strings.ToLower(prompt)
		if strings.Contains(p, "code") || strings.Contains(p, "bug") || strings.Contains(p, "review") {
			return codeAgent
		}
		return chatAgent
	}

	ctx := context.Background()
	for _, prompt := range []string{
		"Please review this code for bugs.",
		"How is your day going?",
	} {
		selected := route(prompt)
		fmt.Printf("Routed %q -> %s\n", prompt, selected.Name())
		resp, err := selected.RunText(ctx, prompt).Collect()
		demo.Response(resp, err)
	}
}
