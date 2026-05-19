// Package server wires the AskUserQuestion tool into an MCP server.
//
// The tool's schema, validation rules, and canonical answer formatting
// come from github.com/dreamware-nz/askuserquestion-go so this package
// stays focused on protocol plumbing.
package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/dreamware-nz/askuserquestion-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resolver is the abstraction the MCP tool handler calls to actually
// surface the question to a human. The default implementation is a
// browser-based picker (internal/browser), but tests and alternative
// frontends can supply their own.
type Resolver interface {
	Ask(ctx context.Context, req askuserquestion.Request) ([]askuserquestion.Answer, error)
}

// Register adds the AskUserQuestion tool to an existing MCP server using
// the canonical SDK tool name. The handler bridges MCP CallTool invocations
// to the supplied Resolver and emits the canonical answer string back as
// the tool result.
func Register(s *mcp.Server, resolver Resolver) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        askuserquestion.SDKToolName,
		Title:       "Ask the user a multiple-choice question",
		Description: askuserquestion.Description(),
	}, handler(resolver))
}

// handler returns the typed MCP ToolHandlerFor that validates inputs,
// hands off to the Resolver, and converts answers back into the SDK
// canonical text result.
//
// Failure modes map onto MCP semantics:
//   - schema violations → result with IsError=true and the validation
//     reason as the text, so the model can self-correct on the next turn
//   - resolver cancellation → non-error result "[cancelled by user]" so
//     the conversation stays valid
//   - context cancellation → propagated up; the SDK will turn this into
//     a protocol-level error and close the call
func handler(resolver Resolver) mcp.ToolHandlerFor[askuserquestion.Params, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in askuserquestion.Params) (*mcp.CallToolResult, any, error) {
		if err := askuserquestion.Validate(in); err != nil {
			return errorResult(err.Error()), nil, nil
		}
		req := askuserquestion.Request{Questions: in.Questions}
		answers, err := resolver.Ask(ctx, req)
		if err != nil {
			if errors.Is(err, askuserquestion.ErrResolverCancelled) {
				return textResult("[cancelled by user]"), nil, nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil, err
			}
			return errorResult(fmt.Sprintf("resolver error: %v", err)), nil, nil
		}
		return textResult(askuserquestion.Format(req, answers)), nil, nil
	}
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}

func errorResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}
