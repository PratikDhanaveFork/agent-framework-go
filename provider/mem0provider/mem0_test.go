// Copyright (c) Microsoft. All rights reserved.

package mem0provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/provider/mem0provider"
)

type capturedRequest struct {
	method string
	path   string
	query  string
	auth   string
	body   map[string]any
}

// mockMem0 records requests and replies with the configured search results.
func mockMem0(t *testing.T, searchMemories []string) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		mu.Lock()
		requests = append(requests, capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		})
		mu.Unlock()

		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/") {
			items := make([]map[string]any, 0, len(searchMemories))
			for _, m := range searchMemories {
				items = append(items, map[string]any{"memory": m})
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func staticScope(s mem0provider.Scope) func(*agent.Session) mem0provider.Scope {
	return func(*agent.Session) mem0provider.Scope { return s }
}

func TestProvider_InjectsRetrievedMemories(t *testing.T) {
	srv, requests := mockMem0(t, []string{"user likes Go", "prefers concise answers"})
	p := mem0provider.NewProvider(srv.URL, staticScope(mem0provider.Scope{UserID: "u1"}), mem0provider.Config{APIKey: "secret"})

	out, _, err := p.Invoking(context.Background(), agent.InvokingContext{
		Messages: []*message.Message{message.NewText("what language should I use?")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected input + 1 injected message, got %d", len(out))
	}
	injected := out[len(out)-1].String()
	if !strings.Contains(injected, "user likes Go") || !strings.Contains(injected, "prefers concise answers") {
		t.Fatalf("injected message missing memories: %q", injected)
	}
	if !strings.Contains(injected, "## Memories") {
		t.Fatalf("injected message missing default context prompt: %q", injected)
	}

	reqs := *requests
	if len(reqs) != 1 || reqs[0].method != http.MethodPost || !strings.HasSuffix(reqs[0].path, "/v1/memories/search/") {
		t.Fatalf("unexpected search request: %+v", reqs)
	}
	if reqs[0].auth != "Token secret" {
		t.Fatalf("Authorization = %q, want %q", reqs[0].auth, "Token secret")
	}
	if reqs[0].body["user_id"] != "u1" {
		t.Fatalf("search body user_id = %v, want u1", reqs[0].body["user_id"])
	}
	if reqs[0].body["query"] != "what language should I use?" {
		t.Fatalf("search body query = %v", reqs[0].body["query"])
	}
}

func TestProvider_NoMemoriesInjectsNothing(t *testing.T) {
	srv, _ := mockMem0(t, nil)
	p := mem0provider.NewProvider(srv.URL, staticScope(mem0provider.Scope{UserID: "u1"}), mem0provider.Config{})

	out, _, err := p.Invoking(context.Background(), agent.InvokingContext{
		Messages: []*message.Message{message.NewText("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected only the input message, got %d", len(out))
	}
}

func TestProvider_SearchFailureDegradesGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	p := mem0provider.NewProvider(srv.URL, staticScope(mem0provider.Scope{UserID: "u1"}), mem0provider.Config{})

	out, _, err := p.Invoking(context.Background(), agent.InvokingContext{
		Messages: []*message.Message{message.NewText("hi")},
	})
	if err != nil {
		t.Fatalf("search failure should degrade gracefully, got error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected only the input message on search failure, got %d", len(out))
	}
}

func TestProvider_StorePersistsConversationalRolesOnly(t *testing.T) {
	srv, requests := mockMem0(t, nil)
	p := mem0provider.NewProvider(srv.URL, staticScope(mem0provider.Scope{AgentID: "a1"}), mem0provider.Config{})

	toolMsg := message.New(&message.FunctionResultContent{CallID: "1", Result: "ignored"})
	toolMsg.Role = message.RoleTool
	err := p.Invoked(context.Background(), agent.InvokedContext{
		RequestMessages:  []*message.Message{message.NewText("remember I use Go")},
		ResponseMessages: []*message.Message{withRole(message.NewText("noted"), message.RoleAssistant), toolMsg},
	})
	if err != nil {
		t.Fatal(err)
	}

	var creates []capturedRequest
	for _, r := range *requests {
		if r.method == http.MethodPost && strings.HasSuffix(r.path, "/v1/memories/") {
			creates = append(creates, r)
		}
	}
	if len(creates) != 2 {
		t.Fatalf("expected 2 create-memory calls (user + assistant), got %d", len(creates))
	}
	// The assistant create should carry role "assistant" and the agent scope.
	found := false
	for _, c := range creates {
		msgs, _ := c.body["messages"].([]any)
		if len(msgs) == 0 {
			t.Fatalf("create body missing messages: %+v", c.body)
		}
		first, _ := msgs[0].(map[string]any)
		if first["role"] == "assistant" && first["content"] == "noted" {
			found = true
		}
		if c.body["agent_id"] != "a1" {
			t.Fatalf("create body agent_id = %v, want a1", c.body["agent_id"])
		}
	}
	if !found {
		t.Fatal("expected an assistant create-memory call for the response message")
	}
}

func TestProvider_ClearStoredMemories(t *testing.T) {
	srv, requests := mockMem0(t, nil)
	p := mem0provider.NewProvider(srv.URL, staticScope(mem0provider.Scope{UserID: "u1", AgentID: "a1"}), mem0provider.Config{})

	if err := p.ClearStoredMemories(context.Background(), &agent.Session{}); err != nil {
		t.Fatal(err)
	}
	reqs := *requests
	if len(reqs) != 1 || reqs[0].method != http.MethodDelete {
		t.Fatalf("expected one DELETE, got %+v", reqs)
	}
	if !strings.Contains(reqs[0].query, "user_id=u1") || !strings.Contains(reqs[0].query, "agent_id=a1") {
		t.Fatalf("clear query missing scope params: %q", reqs[0].query)
	}
}

func TestClearStoredMemories_EmptyScopeErrors(t *testing.T) {
	p := mem0provider.NewProvider("https://example.com", staticScope(mem0provider.Scope{}), mem0provider.Config{})
	if err := p.ClearStoredMemories(context.Background(), &agent.Session{}); err == nil {
		t.Fatal("expected an error clearing an empty scope")
	}
}

func TestNewProvider_Panics(t *testing.T) {
	cases := map[string]func(){
		"empty baseURL": func() {
			mem0provider.NewProvider("", staticScope(mem0provider.Scope{UserID: "u"}), mem0provider.Config{})
		},
		"nil scope": func() { mem0provider.NewProvider("https://example.com", nil, mem0provider.Config{}) },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			fn()
		})
	}
}

func withRole(m *message.Message, role message.Role) *message.Message {
	m.Role = role
	return m
}
