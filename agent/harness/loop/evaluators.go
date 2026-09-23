// Copyright (c) Microsoft. All rights reserved.

package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/microsoft/agent-framework-go/agent/harness/agentmode"
	"github.com/microsoft/agent-framework-go/agent/harness/todo"
)

const (
	// RemainingTodosPlaceholder is the token within a TodoCompletion feedback
	// template that is replaced, on each evaluation, with a formatted list of
	// the remaining (incomplete) todo items.
	RemainingTodosPlaceholder = "{remaining_todos}"

	// DefaultTodoCompletionFeedbackTemplate is used when a TodoCompletion
	// evaluator does not receive a custom feedback template.
	DefaultTodoCompletionFeedbackTemplate = "You still have incomplete todo items. Continue working until every item is complete, marking each item as " +
		"complete when finished. The following items are still open:\n" + RemainingTodosPlaceholder
)

// CompletionMarkerConfig configures a completion-marker evaluator.
type CompletionMarkerConfig struct {
	// Marker is the completion marker that stops the loop when present in the
	// latest response text.
	Marker string

	// FeedbackMessageTemplate is used when the marker is absent. When nil, the
	// default template is used. The
	// completionMarkerPlaceholder token is replaced when the evaluator is
	// created, and lastResponsePlaceholder is replaced on each evaluation.
	FeedbackMessageTemplate *string
}

// CompletionMarkerEvaluator stops the loop once a marker appears in the latest
// response text, otherwise it asks the agent to continue.
type CompletionMarkerEvaluator struct {
	completionMarker        string
	feedbackMessageTemplate string
}

// NewCompletionMarkerEvaluator creates an evaluator that waits for the
// configured marker in the latest response text.
func NewCompletionMarkerEvaluator(config CompletionMarkerConfig) *CompletionMarkerEvaluator {
	marker := strings.TrimSpace(config.Marker)
	if marker == "" {
		panic("loop: completion marker cannot be empty")
	}
	template := defaultCompletionMarkerFeedbackTemplate
	if config.FeedbackMessageTemplate != nil {
		template = *config.FeedbackMessageTemplate
	}
	return &CompletionMarkerEvaluator{
		completionMarker:        marker,
		feedbackMessageTemplate: prepareCompletionMarkerFeedbackTemplate(template, marker),
	}
}

// Evaluate implements Evaluator.
func (e *CompletionMarkerEvaluator) Evaluate(_ context.Context, loop *Context) (Evaluation, error) {
	if loop == nil {
		return Stop(), errors.New("loop: context cannot be nil")
	}
	if loop.LastResponse == nil {
		return Stop(), errors.New("loop: last response cannot be nil")
	}
	responseText := loop.LastResponse.String()
	if strings.Contains(responseText, e.completionMarker) {
		return Stop(), nil
	}
	return Continue(formatCompletionMarkerFeedback(e.feedbackMessageTemplate, responseText)), nil
}

func prepareCompletionMarkerFeedbackTemplate(template, marker string) string {
	return strings.ReplaceAll(template, completionMarkerPlaceholder, marker)
}

func formatCompletionMarkerFeedback(template, responseText string) string {
	return strings.ReplaceAll(template, lastResponsePlaceholder, responseText)
}

// TodoCompletionConfig configures a [TodoCompletionEvaluator].
type TodoCompletionConfig struct {
	// Modes, when non-nil, restricts the evaluator to driving re-invocation
	// only while the session's current mode is one of the listed modes. In any
	// other mode the evaluator stops (declining to drive continuation rather
	// than vetoing other evaluators). When nil, the evaluator applies in every
	// mode and ModeProvider is not required. When non-nil it must contain at
	// least one non-empty mode name and ModeProvider must be supplied.
	Modes []string

	// ModeProvider resolves the session's current mode. It is required when
	// Modes is non-nil and ignored otherwise.
	ModeProvider *agentmode.Provider

	// FeedbackMessageTemplate overrides the feedback produced while incomplete
	// todo items remain. When nil, DefaultTodoCompletionFeedbackTemplate is
	// used. Any occurrence of RemainingTodosPlaceholder is replaced, on each
	// evaluation, with a formatted list of the remaining items; when the
	// placeholder is absent the rendered list is not appended.
	FeedbackMessageTemplate *string
}

// TodoCompletionEvaluator keeps re-invoking the wrapped agent until a
// [todo.Provider] has no remaining (incomplete) items, optionally only while
// the agent is operating in one of a configured set of modes.
//
// Unlike the .NET evaluator, which resolves its providers from the agent via
// reflection, the Go evaluator takes the [todo.Provider] (and, when modes are
// configured, the [agentmode.Provider]) explicitly and reads the run session
// from [Context.Session].
type TodoCompletionEvaluator struct {
	todos                   *todo.Provider
	modeProvider            *agentmode.Provider
	modes                   map[string]struct{}
	feedbackMessageTemplate string
}

// NewTodoCompletionEvaluator creates an evaluator that re-invokes the agent
// until the given todo provider reports no remaining items. It panics if todos
// is nil, if config.Modes is non-nil but empty or contains a blank name, or if
// modes are configured without a ModeProvider.
func NewTodoCompletionEvaluator(todos *todo.Provider, config TodoCompletionConfig) *TodoCompletionEvaluator {
	if todos == nil {
		panic("loop: todo provider cannot be nil")
	}
	e := &TodoCompletionEvaluator{
		todos:                   todos,
		feedbackMessageTemplate: DefaultTodoCompletionFeedbackTemplate,
	}
	if config.FeedbackMessageTemplate != nil {
		e.feedbackMessageTemplate = *config.FeedbackMessageTemplate
	}
	if config.Modes != nil {
		if config.ModeProvider == nil {
			panic("loop: ModeProvider is required when Modes is configured")
		}
		modes := make(map[string]struct{}, len(config.Modes))
		for _, mode := range config.Modes {
			if strings.TrimSpace(mode) == "" {
				panic("loop: mode names must not be empty or whitespace")
			}
			modes[mode] = struct{}{}
		}
		if len(modes) == 0 {
			panic("loop: at least one mode must be supplied when Modes is configured")
		}
		e.modes = modes
		e.modeProvider = config.ModeProvider
	}
	return e
}

// Evaluate implements Evaluator.
func (e *TodoCompletionEvaluator) Evaluate(_ context.Context, loop *Context) (Evaluation, error) {
	if loop == nil {
		return Stop(), errors.New("loop: context cannot be nil")
	}

	// When modes are configured, only drive re-invocation while the current
	// mode is one of them.
	if e.modes != nil {
		currentMode := e.modeProvider.ModeForSession(loop.Session)
		if _, ok := e.modes[currentMode]; !ok {
			return Stop(), nil
		}
	}

	remaining := e.todos.RemainingTodos(loop.Session)
	if len(remaining) == 0 {
		return Stop(), nil
	}
	feedback := strings.ReplaceAll(e.feedbackMessageTemplate, RemainingTodosPlaceholder, formatRemainingTodos(remaining))
	return Continue(feedback), nil
}

func formatRemainingTodos(remaining []todo.Item) string {
	var builder strings.Builder
	for i, item := range remaining {
		if i > 0 {
			builder.WriteByte('\n')
		}
		fmt.Fprintf(&builder, "- %d: %s", item.ID, item.Title)
		if strings.TrimSpace(item.Description) != "" {
			builder.WriteString(" — ")
			builder.WriteString(item.Description)
		}
	}
	return builder.String()
}
