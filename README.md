# askuserquestion-mcp

A stdio MCP server exposing Claude Code's `AskUserQuestion` tool to any
MCP-aware host (Crush, Cursor, Claude Desktop, custom agents).

The user is prompted via a small browser-rendered form on an ephemeral
loopback port. Selections flow back as the canonical SDK answer string:

```
Auth method?
OAuth

Languages?
- Go
- Rust
```

## Why this exists

Some MCP hosts (notably Crush at the time of writing) do not yet wire
the MCP `elicitation/create` server→client request, so an MCP server
cannot delegate "please ask the user" back up to the host. This package
sidesteps that by hosting the picker itself: a tiny loopback HTTP server,
an embedded HTML form, no external dependencies on the host's UI.

When hosts eventually implement elicitation, the Resolver seam in
`internal/server` makes it a small follow-up to ship an alternative
backend that issues `elicitation/create` instead of opening a browser.

## Install

```sh
go install github.com/dreamware-nz/askuserquestion-mcp/cmd/askuserquestion-mcp@latest
```

Requires Go 1.26+ (matching the `charm.land/fantasy` / `askuserquestion-go`
baseline).

## Use with Crush

Drop this into your `crush.json`:

```json
{
  "mcp": {
    "askuserquestion": {
      "type": "stdio",
      "command": "askuserquestion-mcp"
    }
  }
}
```

That's it. Crush will list the server's tool as
`mcp_askuserquestion_AskUserQuestion` and route invocations to this
binary. When the model calls it, your browser will open with the
question form; click submit and the answer flows back to the agent.

### Flags

| Flag         | Default     | What it does                                                                  |
| ------------ | ----------- | ----------------------------------------------------------------------------- |
| `--host`     | `127.0.0.1` | Bind address for the form server. Use `localhost` or pin to a specific NIC.   |
| `--no-open`  | off         | Print the URL on stderr instead of trying to launch the browser. Useful in    |
|              |             | headless / SSH sessions.                                                      |

Logs go to stderr; the JSON-RPC stream owns stdout.

## Tool surface

Single tool: `AskUserQuestion`. Schema, validation, and answer formatting
come from [`dreamware-nz/askuserquestion-go`](https://github.com/dreamware-nz/askuserquestion-go);
see that repo for the wire format and limits (1-4 questions, 2-4 options
each, header ≤ 12 chars).

## How it works

```
LLM ──tool call──▶ MCP host ──stdio──▶ askuserquestion-mcp
                                              │
                                              ├─ validate input
                                              ├─ bind ephemeral :0 on 127.0.0.1
                                              ├─ render embedded HTML form
                                              ├─ open browser (or print URL)
                                              ├─ block on POST /submit
                                              └─ format answers → CallToolResult ──▶ host ──▶ LLM
```

Only one Ask runs at a time per process; concurrent calls serialize on
the Resolver mutex. The HTTP server is torn down at the end of each
call so nothing is left listening between turns.

## Cancellation semantics

- **User closes the browser tab** → no signal until ctx times out. The
  agent context typically cancels the call when the user cancels the
  turn, which propagates correctly.
- **Resolver returns `ErrResolverCancelled`** → non-error tool result
  `[cancelled by user]` so the conversation stays valid.
- **Validation failure** → tool result with `IsError=true` so the model
  can self-correct on the next turn.

## Security

- Listener binds to `127.0.0.1` by default; never exposed beyond loopback
  unless you pass `--host`.
- Form payload is read once per Ask; the server shuts down immediately
  after.
- No persistence, no caching, no auth (loopback-only).

## License

MIT.
