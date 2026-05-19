// Command askuserquestion-mcp is a stdio MCP server that exposes Claude
// Code's AskUserQuestion tool. The user is prompted through a browser
// form served on an ephemeral localhost port; selections flow back as
// the canonical SDK answer string.
//
// Usage with Crush, Cursor, or any MCP-aware host:
//
//	{
//	  "mcp": {
//	    "askuserquestion": {
//	      "type": "stdio",
//	      "command": "askuserquestion-mcp"
//	    }
//	  }
//	}
//
// No flags are required. Logs go to stderr so they don't pollute the
// JSON-RPC stream on stdout.
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dreamware-nz/askuserquestion-mcp/internal/browser"
	"github.com/dreamware-nz/askuserquestion-mcp/internal/server"
)

// serverInstructions is surfaced to MCP clients during initialization. Most
// hosts inject these instructions into the model's system context, so this
// is the most reliable place to nudge the model toward actually calling the
// tool instead of guessing, fabricating defaults, or stalling on ambiguity.
//
// The prose is deliberately blunt: models tend to "be helpful" by inferring
// answers, which silently bypasses the human in the loop. Naming the tool
// explicitly (both spellings) and listing concrete trigger situations makes
// invocation the obvious next step when uncertainty appears.
const serverInstructions = `This server exposes a single tool, AskUserQuestion (alias: ask_user_question), for putting a multiple-choice question to the human user and waiting for their answer.

USE THIS TOOL — do not guess, assume defaults, or stall — whenever any of the following is true:
  - the user's request is ambiguous or under-specified
  - there are two or more reasonable ways to proceed and the choice changes the outcome
  - a destructive or irreversible action is about to be taken
  - you need a preference, credential choice, target environment, file path, name, or scope decision
  - you would otherwise write "I'll assume...", "by default I'll...", or "let me know if..."

Prefer calling AskUserQuestion over asking in plain prose: it renders a real picker UI for the user and returns a canonical machine-readable answer.

Rules of thumb:
  - 1-4 questions per call, batched together when they're related
  - 2-4 options per question, each with a short Header (<=12 chars) for the picker chip
  - put the recommended option first and suffix its label with " (Recommended)"
  - the user can always type a free-form "Other" answer, so options don't need to be exhaustive
  - set multiSelect: true only when more than one answer genuinely makes sense

If the tool returns "[cancelled by user]" the user dismissed the prompt; treat that as "no answer given" and proceed conservatively or ask again in prose.`

func main() {
	noOpen := flag.Bool("no-open", false, "Do not auto-open the browser; print the URL on stderr instead.")
	host := flag.String("host", "127.0.0.1", "Bind address for the browser form server.")
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("askuserquestion-mcp: ")
	log.SetOutput(os.Stderr)

	resolver := browser.New()
	resolver.Host = *host
	resolver.Logger = os.Stderr
	if *noOpen {
		resolver.OpenURL = nil
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "askuserquestion-mcp",
		Title:   "AskUserQuestion",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	server.Register(s, resolver)

	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
