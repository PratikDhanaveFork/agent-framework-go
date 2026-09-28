// Copyright (c) Microsoft. All rights reserved.

// Package mem0provider provides an [agent.ContextProvider] backed by the Mem0
// memory service (https://mem0.ai). It searches Mem0 for memories relevant to
// the current request and injects them into the model context, and persists
// user, assistant, and system messages as new memories after each invocation.
//
// # Security
//
// This provider sends conversation content — including user input, model
// output, and system instructions — to an external Mem0 service over HTTP, and
// injects the service's responses into the model context without sanitization.
// A compromised memory store is therefore an indirect prompt-injection vector.
// Configure the provider to use HTTPS and appropriate authentication, apply
// suitable data-retention and access controls on the Mem0 side, and trust the
// memory store accordingly.
package mem0provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
)

// defaultContextPrompt prefixes the retrieved memories injected into the model
// context. It mirrors the .NET Mem0Provider default.
const defaultContextPrompt = "## Memories\nConsider the following memories when answering user questions:"

// Scope identifies the Mem0 partitions a provider reads from and writes to.
// At least one field must be non-empty for a search, store, or clear to run.
type Scope struct {
	ApplicationID string
	AgentID       string
	ThreadID      string
	UserID        string
}

func (s Scope) isEmpty() bool {
	return s.ApplicationID == "" && s.AgentID == "" && s.ThreadID == "" && s.UserID == ""
}

// Config configures a Mem0 [Provider].
type Config struct {
	// APIKey, when set, is sent as an "Authorization: Token <APIKey>" header on
	// every request. Leave empty to authenticate via a custom HTTPClient
	// transport instead.
	APIKey string

	// HTTPClient issues the Mem0 requests. When nil, [http.DefaultClient] is
	// used. Configure its transport for authentication when APIKey is not set.
	HTTPClient *http.Client

	// ContextPrompt prefixes the retrieved memories injected into the model
	// context. When nil, a default prompt is used.
	ContextPrompt *string

	// SearchScope resolves the scope used when searching for memories. When nil,
	// the storage scope passed to [NewProvider] is used for searching as well.
	SearchScope func(*agent.Session) Scope

	// SearchInputFilter selects which request messages contribute to the search
	// query. When nil, all provided request messages are used.
	SearchInputFilter func([]*message.Message) []*message.Message
}

// Provider is an [agent.ContextProvider] backed by the Mem0 memory service.
// Use [NewProvider] to create one; it can be passed directly to an agent's
// context providers.
type Provider struct {
	provider      agent.ContextProvider
	client        *client
	scope         func(*agent.Session) Scope
	searchScope   func(*agent.Session) Scope
	contextPrompt string
	searchFilter  func([]*message.Message) []*message.Message
}

// NewProvider creates a Mem0 provider. baseURL is the Mem0 service base URL
// (for example "https://api.mem0.ai"); scope resolves the storage — and, unless
// [Config.SearchScope] is set, the search — partition for a session. It panics
// if baseURL is empty or scope is nil.
func NewProvider(baseURL string, scope func(*agent.Session) Scope, config Config) *Provider {
	if strings.TrimSpace(baseURL) == "" {
		panic("mem0provider: baseURL is required")
	}
	if scope == nil {
		panic("mem0provider: scope function is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	prompt := defaultContextPrompt
	if config.ContextPrompt != nil {
		prompt = *config.ContextPrompt
	}
	p := &Provider{
		client:        &client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, apiKey: config.APIKey},
		scope:         scope,
		searchScope:   config.SearchScope,
		contextPrompt: prompt,
		searchFilter:  config.SearchInputFilter,
	}
	p.provider = agent.NewContextProvider(agent.ContextProviderConfig{
		SourceID: "Mem0Provider",
		Provide:  p.provide,
		Store:    p.store,
	})
	return p
}

// Invoking implements [agent.ContextProvider]. It searches Mem0 for memories
// relevant to the request and injects them as an additional context message.
func (p *Provider) Invoking(ctx context.Context, invoking agent.InvokingContext) ([]*message.Message, []agent.Option, error) {
	return p.provider.Invoking(ctx, invoking)
}

// Invoked implements [agent.ContextProvider]. It persists request and response
// messages to Mem0 as new memories.
func (p *Provider) Invoked(ctx context.Context, invoked agent.InvokedContext) error {
	return p.provider.Invoked(ctx, invoked)
}

// ClearStoredMemories deletes every memory stored under the session's storage
// scope. It returns an error if the scope is empty or the service call fails.
func (p *Provider) ClearStoredMemories(ctx context.Context, session *agent.Session) error {
	scope := p.scope(session)
	if scope.isEmpty() {
		return errors.New("mem0provider: cannot clear memories for an empty scope")
	}
	return p.client.clear(ctx, scope)
}

func (p *Provider) resolveSearchScope(session *agent.Session) Scope {
	if p.searchScope != nil {
		return p.searchScope(session)
	}
	return p.scope(session)
}

// provide searches Mem0 and, when memories are found, returns a single context
// message prefixed by the configured prompt. A search failure degrades to
// injecting no memories rather than failing the run, matching the .NET
// provider; the request continues without memory context.
func (p *Provider) provide(ctx context.Context, invoking agent.InvokingContext) ([]*message.Message, []agent.Option, error) {
	session, _ := agent.GetOption(invoking.Options, agent.WithSession)
	scope := p.resolveSearchScope(session)
	if scope.isEmpty() {
		return nil, nil, nil
	}

	msgs := invoking.Messages
	if p.searchFilter != nil {
		msgs = p.searchFilter(msgs)
	}
	query := joinMessageText(msgs)

	memories, err := p.client.search(ctx, scope, query)
	if err != nil || len(memories) == 0 {
		return nil, nil, nil
	}
	text := p.contextPrompt + "\n" + strings.Join(memories, "\n")
	return []*message.Message{message.NewText(text)}, nil, nil
}

// store persists user, assistant, and system messages from the invocation as
// new memories. A persistence failure degrades to a no-op rather than failing
// the run, matching the .NET provider.
func (p *Provider) store(ctx context.Context, invoked agent.InvokedContext) error {
	session, _ := agent.GetOption(invoked.Options, agent.WithSession)
	scope := p.scope(session)
	if scope.isEmpty() {
		return nil
	}

	msgs := make([]*message.Message, 0, len(invoked.RequestMessages)+len(invoked.ResponseMessages))
	msgs = append(msgs, invoked.RequestMessages...)
	msgs = append(msgs, invoked.ResponseMessages...)
	for _, m := range msgs {
		if m == nil {
			continue
		}
		switch m.Role {
		case message.RoleUser, message.RoleAssistant, message.RoleSystem:
		default:
			continue // Mem0 only stores conversational roles.
		}
		text := strings.TrimSpace(m.String())
		if text == "" {
			continue
		}
		if err := p.client.createMemory(ctx, scope, text, strings.ToLower(string(m.Role))); err != nil {
			return nil
		}
	}
	return nil
}

func joinMessageText(msgs []*message.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if text := strings.TrimSpace(m.String()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// client is a minimal Mem0 REST client.
type client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

type searchRequest struct {
	AppID   string `json:"app_id,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Query   string `json:"query"`
}

type searchResponseItem struct {
	Memory string `json:"memory"`
}

type createMemoryRequest struct {
	AppID    string                `json:"app_id,omitempty"`
	AgentID  string                `json:"agent_id,omitempty"`
	RunID    string                `json:"run_id,omitempty"`
	UserID   string                `json:"user_id,omitempty"`
	Messages []createMemoryMessage `json:"messages"`
}

type createMemoryMessage struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

func (c *client) search(ctx context.Context, scope Scope, query string) ([]string, error) {
	body := searchRequest{
		AppID:   scope.ApplicationID,
		AgentID: scope.AgentID,
		RunID:   scope.ThreadID,
		UserID:  scope.UserID,
		Query:   query,
	}
	var items []searchResponseItem
	if err := c.do(ctx, http.MethodPost, "/v1/memories/search/", body, &items); err != nil {
		return nil, err
	}
	memories := make([]string, 0, len(items))
	for _, item := range items {
		if item.Memory != "" {
			memories = append(memories, item.Memory)
		}
	}
	return memories, nil
}

func (c *client) createMemory(ctx context.Context, scope Scope, content, role string) error {
	body := createMemoryRequest{
		AppID:    scope.ApplicationID,
		AgentID:  scope.AgentID,
		RunID:    scope.ThreadID,
		UserID:   scope.UserID,
		Messages: []createMemoryMessage{{Content: content, Role: role}},
	}
	return c.do(ctx, http.MethodPost, "/v1/memories/", body, nil)
}

func (c *client) clear(ctx context.Context, scope Scope) error {
	q := url.Values{}
	if scope.ApplicationID != "" {
		q.Set("app_id", scope.ApplicationID)
	}
	if scope.AgentID != "" {
		q.Set("agent_id", scope.AgentID)
	}
	if scope.ThreadID != "" {
		q.Set("run_id", scope.ThreadID)
	}
	if scope.UserID != "" {
		q.Set("user_id", scope.UserID)
	}
	return c.do(ctx, http.MethodDelete, "/v1/memories/?"+q.Encode(), nil, nil)
}

func (c *client) do(ctx context.Context, method, path string, reqBody, out any) error {
	var reader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Token "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mem0provider: %s %s: unexpected status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
