// Copyright (c) Microsoft. All rights reserved.

// This example shows how to write a custom agent.Middleware. Middleware wraps a
// run and can inspect or transform the messages and options before the provider
// sees them (and the updates on the way back). Here a redaction middleware
// rewrites a secret token in the user's messages before they reach the
// provider. The provider is a deterministic echo so the redaction is visible in
// the output; the example needs no credentials.
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
	"Custom Middleware",
	"Demonstrates a request-transforming agent.Middleware (secret redaction).",
	"Model", "echo-1",
)

// redactMiddleware returns a middleware that replaces every occurrence of
// secret in the user's text content with "[REDACTED]" before the wrapped run
// (and ultimately the provider) sees the messages. It rebuilds the text content
// rather than mutating the caller's, so the original messages are untouched.
func redactMiddleware(secret string) agent.Middleware {
	return agent.MiddlewareFunc(func(next agent.RunFunc, ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
		redacted := make([]*message.Message, 0, len(messages))
		for _, msg := range messages {
			clone := msg.Clone()
			newContents := make(message.Contents, len(clone.Contents))
			for i, c := range clone.Contents {
				if tc, ok := c.(*message.TextContent); ok {
					newContents[i] = &message.TextContent{Text: strings.ReplaceAll(tc.Text, secret, "[REDACTED]")}
				} else {
					newContents[i] = c
				}
			}
			clone.Contents = newContents
			redacted = append(redacted, clone)
		}
		return next(ctx, redacted, options...)
	})
}

// echoRun is a deterministic provider that echoes the latest user message it
// receives, so whatever the middleware passed through is visible.
func echoRun(_ context.Context, messages []*message.Message, _ ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
	return func(yield func(*agent.ResponseUpdate, error) bool) {
		var last string
		for _, msg := range messages {
			if msg.Role == message.RoleUser {
				last = msg.String()
			}
		}
		yield(&agent.ResponseUpdate{
			Role:     message.RoleAssistant,
			Contents: message.Contents{&message.TextContent{Text: "received: " + last}},
		}, nil)
	}
}

func main() {
	a := agent.New(
		agent.ProviderConfig{ProviderName: "echo", Run: echoRun},
		agent.Config{
			Name: "Echoer",
			// The redaction middleware runs before the provider; the logger
			// records the interaction.
			Middlewares: []agent.Middleware{redactMiddleware("hunter2"), logger},
		},
	)

	resp, err := a.RunText(context.Background(), "My password is hunter2, please remember it.").Collect()
	demo.Response(resp, err)
}
