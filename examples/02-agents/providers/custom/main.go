// Copyright (c) Microsoft. All rights reserved.

// This example shows how to back an agent with a custom provider by
// implementing agent.ProviderConfig directly, rather than using one of the
// bundled providers. Any backend — a bespoke HTTP service, a local model, or
// in this case a deterministic in-process transform — can be adapted by
// supplying a Run function that streams agent.ResponseUpdate values.
//
// Streaming is the provider primitive: the framework collects the streamed
// updates into a Response, so a provider only needs to yield updates. This
// example needs no credentials and produces deterministic output.
package main

import (
	"context"
	"iter"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/examples/internal/demo"
	"github.com/microsoft/agent-framework-go/message"
)

var logger = demo.NewLogger(
	"Custom Provider",
	"Demonstrates backing an agent with a custom agent.ProviderConfig.Run function.",
	"Model", "shout-1",
)

// shoutRun is a custom provider implementation. It reads the latest user
// message and streams back an upper-cased reply one word at a time, showing
// that a provider's only obligation is to yield agent.ResponseUpdate values.
func shoutRun(_ context.Context, messages []*message.Message, _ ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
	return func(yield func(*agent.ResponseUpdate, error) bool) {
		var prompt string
		for _, msg := range messages {
			if msg.Role == message.RoleUser {
				prompt = msg.String()
			}
		}
		for i, word := range strings.Fields(strings.ToUpper(prompt)) {
			text := word
			if i > 0 {
				text = " " + text
			}
			if !yield(&agent.ResponseUpdate{
				Role:     message.RoleAssistant,
				Contents: message.Contents{&message.TextContent{Text: text}},
			}, nil) {
				return
			}
		}
	}
}

func main() {
	// Wire the custom Run function into an agent. ProviderConfig is the only
	// thing that ties an agent to a backend.
	a := agent.New(
		agent.ProviderConfig{ProviderName: "custom-shout", Run: shoutRun},
		agent.Config{
			Name:        "Shouter",
			Middlewares: []agent.Middleware{logger}, // for logging agent interactions
		},
	)

	ctx := context.Background()

	// Non-streaming: the framework collects the streamed words into one Response.
	resp, err := a.RunText(ctx, "hello from a custom provider").Collect()
	demo.Response(resp, err)

	// Streaming: the same updates are surfaced incrementally.
	for update, err := range a.RunText(ctx, "streaming works too", agent.Stream(true)) {
		demo.Response(update, err)
	}
}
