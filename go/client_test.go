package kimi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreatesSessionAndStreamsPromptEvents(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	server := writeFakeServer(t)
	client, err := Connect(context.Background(),
		WithExecutable(python),
		WithArgs(server),
		WithClientName("go-client-test"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())

	session, err := client.CreateSession(context.Background(),
		WithSessionID("ses_go_test"),
		WithWorkDir("/tmp/project"),
		WithThinking(false),
		WithPermission(PermissionAuto),
	)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID() != "ses_go_test" {
		t.Fatalf("session.ID() = %q", session.ID())
	}

	events, err := session.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	var got []Event
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Type != "turn.started" || got[1].Delta != "hello from fake server" || got[2].Reason != "completed" {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestMapsProtocolErrors(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	server := writeFakeServer(t)
	client, err := Connect(context.Background(), WithExecutable(python), WithArgs(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())

	_, err = client.CreateSession(context.Background(), WithWorkDir("/tmp/missing"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, "INVALID_INPUT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeFakeServer(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "fake_server.py")
	err := os.WriteFile(file, []byte(`
import json
import sys

def write(message):
    sys.stdout.write(json.dumps(message, separators=(",", ":")) + "\n")
    sys.stdout.flush()

for line in sys.stdin:
    if not line.strip():
        continue
    request = json.loads(line)
    if request["method"] == "initialize":
        write({
            "jsonrpc": "2.0",
            "id": request["id"],
            "result": {
                "protocolVersion": "1.0",
                "supportedVersions": ["1.0"],
                "server": {"name": "kimi-code", "version": "0.0.0-test"},
                "capabilities": {},
            },
        })
        continue
    if request["method"] == "createSession":
        if request["params"].get("id") != "ses_go_test":
            write({
                "jsonrpc": "2.0",
                "id": request["id"],
                "error": {
                    "code": "INVALID_INPUT",
                    "message": "id must be ses_go_test",
                    "data": {"retryable": False},
                },
            })
            continue
        write({
            "jsonrpc": "2.0",
            "id": request["id"],
            "result": {
                "id": "ses_go_test",
                "workDir": request["params"]["workDir"],
                "sessionDir": "/tmp/session",
                "createdAt": 1,
                "updatedAt": 2,
            },
        })
        continue
    if request["method"] == "prompt":
        write({
            "jsonrpc": "2.0",
            "method": "event",
            "params": {
                "type": "turn.started",
                "sessionId": request["params"]["sessionId"],
                "agentId": "main",
                "turnId": 0,
            },
        })
        write({
            "jsonrpc": "2.0",
            "method": "event",
            "params": {
                "type": "assistant.delta",
                "sessionId": request["params"]["sessionId"],
                "agentId": "main",
                "turnId": 0,
                "delta": "hello from fake server",
            },
        })
        write({
            "jsonrpc": "2.0",
            "method": "event",
            "params": {
                "type": "turn.ended",
                "sessionId": request["params"]["sessionId"],
                "agentId": "main",
                "turnId": 0,
                "reason": "completed",
            },
        })
        write({"jsonrpc": "2.0", "id": request["id"], "result": {"turnId": 0}})
        continue
    if request["method"] == "shutdown":
        write({"jsonrpc": "2.0", "id": request["id"], "result": {}})
        raise SystemExit(0)
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
