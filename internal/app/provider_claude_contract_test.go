package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Real CLI / deterministic transport contract, not a real-model qualification.
// Run only with the checksum-verified native distribution named below, in an
// isolated container with no external network or real credentials. The server
// is loopback and plays the model: it answers through the structured-output
// tool, then ends the turn.
const claudeContractVersion = "2.1.325 (Claude Code)"

func TestProviderClaudeRealCLIContract(t *testing.T) {
	path := os.Getenv("AUROSCOPE_TEST_REAL_CLAUDE")
	if path == "" {
		t.Skip("set AUROSCOPE_TEST_REAL_CLAUDE to verified " + claudeContractVersion)
	}
	version, err := exec.Command(path, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != claudeContractVersion {
		t.Fatalf("expected %s, got %q", claudeContractVersion, strings.TrimSpace(string(version)))
	}
	t.Setenv("AUROSCOPE_CLAUDE", path)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "inert-local-contract-key")
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		data, _ := io.ReadAll(r.Body)
		var request struct {
			Tools []struct {
				Name        string          `json:"name"`
				InputSchema json.RawMessage `json:"input_schema"`
			} `json:"tools"`
			Stream    bool   `json:"stream"`
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		if json.Unmarshal(data, &request) != nil {
			t.Error("invalid request")
			w.WriteHeader(400)
			return
		}
		// Only the answer-recording tool may be offered: no shell, file,
		// web or MCP capability reaches the model.
		if len(request.Tools) != 1 || request.Tools[0].Name != "StructuredOutput" {
			names := []string{}
			for _, tool := range request.Tools {
				names = append(names, tool.Name)
			}
			t.Errorf("real CLI offered model tools %v", names)
			w.WriteHeader(400)
			return
		}
		if !bytes.Contains(request.Tools[0].InputSchema, []byte(`"inspect"`)) {
			t.Error("structured-output tool does not carry the audit schema")
		}
		if !bytes.Contains(data, []byte("pkgname=contract-fixture")) {
			t.Error("real CLI did not send bundle")
			w.WriteHeader(400)
			return
		}
		call := calls.Add(1)
		answered := bytes.Contains(data, []byte(`"tool_result"`))
		t.Logf("local request %d: stream=%v model=%s max_tokens=%d answered=%v", call, request.Stream, request.Model, request.MaxTokens, answered)
		var blocks []map[string]any
		stop := "end_turn"
		if answered {
			blocks = []map[string]any{{"type": "text", "text": "done"}}
		} else {
			stop = "tool_use"
			blocks = []map[string]any{{"type": "tool_use", "id": "toolu_fixture", "name": "StructuredOutput", "input": json.RawMessage(providerTestReport)}}
		}
		if !request.Stream {
			data, _ := json.Marshal(map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "model": request.Model, "content": blocks, "stop_reason": stop, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 10, "output_tokens": 30}})
			w.Write(data)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []any{map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "model": request.Model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 10, "output_tokens": 0}}}}
		for index, block := range blocks {
			if block["type"] == "tool_use" {
				events = append(events,
					map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]any{"type": "tool_use", "id": block["id"], "name": block["name"], "input": map[string]any{}}},
					map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]string{"type": "input_json_delta", "partial_json": providerTestReport}})
			} else {
				events = append(events,
					map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]string{"type": "text", "text": ""}},
					map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]string{"type": "text_delta", "text": block["text"].(string)}})
			}
			events = append(events, map[string]any{"type": "content_block_stop", "index": index})
		}
		events = append(events,
			map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 30}},
			map[string]any{"type": "message_stop"})
		for _, event := range events {
			data, _ := json.Marshal(event)
			kind := event.(map[string]any)["type"]
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
		}
	}))
	defer s.Close()
	t.Setenv("ANTHROPIC_BASE_URL", s.URL)
	bundle := auditBundle{Identity: recipeIdentity{Pkgbase: "contract-fixture"}, Files: []recipeFile{{Path: "PKGBUILD", Text: "pkgname=contract-fixture\n"}}}
	config := runConfig{codexTimeout: 60 * time.Second, stdout: io.Discard, stderr: io.Discard}.withDefaults()
	report, err := auditClaude(bundle, auditConfig{Model: "claude-sonnet-4-6"}, config)
	if err != nil {
		t.Fatalf("production CLI client: %v; local message requests=%d", err, calls.Load())
	}
	if report.Summary != "recipe reviewed" || calls.Load() < 1 || calls.Load() > 2 {
		t.Fatalf("unexpected result or request count: %d", calls.Load())
	}
	t.Logf("%s: production argv accepted, isolated stdin bundle, only StructuredOutput offered, %d simulated requests, structured output validated", claudeContractVersion, calls.Load())
}
