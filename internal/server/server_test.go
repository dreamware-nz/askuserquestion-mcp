package server

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamware-nz/askuserquestion-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubResolver struct {
	answers []askuserquestion.Answer
	err     error
	called  int
}

func (s *stubResolver) Ask(_ context.Context, _ askuserquestion.Request) ([]askuserquestion.Answer, error) {
	s.called++
	return s.answers, s.err
}

func validParams(t *testing.T) askuserquestion.Params {
	t.Helper()
	return askuserquestion.Params{Questions: []askuserquestion.Question{{
		Question: "Pick?",
		Header:   "Pick",
		Options:  []askuserquestion.Option{{Label: "A"}, {Label: "B"}},
	}}}
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("nil or empty result content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestHandlerHappyPath(t *testing.T) {
	t.Parallel()
	res, _, err := handler(&stubResolver{
		answers: []askuserquestion.Answer{{Selected: []string{"A"}}},
	})(context.Background(), nil, validParams(t))
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %+v", res)
	}
	if got, want := textOf(t, res), "Pick?\nA"; got != want {
		t.Fatalf("text = %q want %q", got, want)
	}
}

func TestHandlerValidationFailure(t *testing.T) {
	t.Parallel()
	stub := &stubResolver{}
	res, _, err := handler(stub)(context.Background(), nil, askuserquestion.Params{})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError on empty Params: %+v", res)
	}
	if stub.called != 0 {
		t.Fatalf("resolver must not be called on invalid input")
	}
}

func TestHandlerCancelled(t *testing.T) {
	t.Parallel()
	res, _, err := handler(&stubResolver{err: askuserquestion.ErrResolverCancelled})(
		context.Background(), nil, validParams(t),
	)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if res.IsError {
		t.Fatalf("cancel must surface as non-error result: %+v", res)
	}
	if got, want := textOf(t, res), "[cancelled by user]"; got != want {
		t.Fatalf("text = %q want %q", got, want)
	}
}

func TestHandlerContextCancel(t *testing.T) {
	t.Parallel()
	_, _, err := handler(&stubResolver{err: context.Canceled})(
		context.Background(), nil, validParams(t),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRegisterAdvertisesTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	Register(s, &stubResolver{})

	cTransport, sTransport := mcp.NewInMemoryTransports()
	srvSession, err := s.Connect(ctx, sTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer srvSession.Close()

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cliSession, err := c.Connect(ctx, cTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer cliSession.Close()

	res, err := cliSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == askuserquestion.SDKToolName {
			return
		}
	}
	t.Fatalf("AskUserQuestion not advertised; got %d tools", len(res.Tools))
}
