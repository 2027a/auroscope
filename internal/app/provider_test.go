package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const providerTestReport = `{"summary":"recipe reviewed","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}`

// Claude Code returns the schema-constrained report in structured_output; the
// text result is prose, here deliberately fenced Markdown that is not a report.
func claudeTestEnvelope() map[string]any {
	return map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "```json\n" + providerTestReport + "\n```", "structured_output": json.RawMessage(providerTestReport)}
}

func writeProviderConfig(t *testing.T, text string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "auroscope")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProviderConfiguration(t *testing.T) {
	for _, tc := range []struct {
		text, provider string
		valid          bool
	}{
		{`{}`, "codex", true},
		{`{"provider":"codex"}`, "codex", true},
		{`{"provider":"claude-code","model":"sonnet"}`, "claude-code", true},
		{`{"provider":"anthropic","model":"example-model"}`, "anthropic", true},
		{`{"provider":"openai","model":"example-model"}`, "openai", true},
		{`{"provider":"openai-compatible","model":"example/model","api_key_env":"TEST_KEY","base_url":"https://example.invalid/api/v1/"}`, "openai-compatible", true},
		{`{"provider":"disabled"}`, "", false},
		{`{"provider":"openai"}`, "", false},
		{`{"provider":"openai","model":"x","base_url":"https://elsewhere.invalid"}`, "", false},
		{`{"provider":"codex","api_key_env":"TEST_KEY"}`, "", false},
		{`{"provider":"openai","model":"x","api_key":"secret-value"}`, "", false},
		{`{"provider":"openai","model":"x","api_key_env":"secret-value"}`, "", false},
		{`{"provider":"codex","provider":"claude-code"}`, "", false},
		{`{"provider": "codex"} {}`, "", false},
		{`null`, "", false},
		{`{"provider":"secret-value"}`, "", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			path := writeProviderConfig(t, tc.text)
			c, err := loadAuditConfig()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && c.Provider != tc.provider {
				t.Fatal(c.Provider)
			}
			if err != nil && strings.Contains(err.Error(), "secret-value") {
				t.Fatal("secret disclosed")
			}
			if got := readString(t, path); got != tc.text {
				t.Fatal("configuration rewritten")
			}
		})
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := loadAuditConfig()
	if err != nil || c.Provider != "codex" {
		t.Fatalf("default: %#v %v", c, err)
	}
	for _, url := range []string{"http://localhost/v1", "https://user:secret-value@example.invalid", "https://example.invalid?key=secret-value", "https://example.invalid/#secret-value", ""} {
		_, err := (auditConfig{Provider: "openai-compatible", Model: "model", APIKeyEnv: "TEST_KEY", BaseURL: url}).normalized()
		if err == nil || strings.Contains(err.Error(), "secret-value") {
			t.Fatalf("unsafe endpoint accepted/disclosed: %v", err)
		}
	}
}

func providerEnvelope(provider, report string) string {
	var result any
	if provider == "anthropic" {
		result = map[string]any{"type": "message", "role": "assistant", "stop_reason": "end_turn", "content": []map[string]string{{"type": "text", "text": report}}}
	} else {
		result = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": report, "refusal": nil}}}}
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func TestProviderHTTPContracts(t *testing.T) {
	for _, provider := range []string{"openai", "openai-compatible", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("TEST_KEY", "credential-fixture")
			var calls atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" {
					t.Error("not POST")
				}
				expectedPath := "/v1/chat/completions"
				if provider == "anthropic" {
					expectedPath = "/v1/messages"
					if r.Header.Get("x-api-key") != "credential-fixture" || r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("Anthropic headers")
					}
				} else if r.Header.Get("Authorization") != "Bearer credential-fixture" {
					t.Error("OpenAI auth")
				}
				if r.URL.Path != expectedPath {
					t.Error(r.URL.Path)
				}
				data, _ := io.ReadAll(r.Body)
				if strings.Contains(string(data), "credential-fixture") {
					t.Error("key in model input")
				}
				var request map[string]any
				if json.Unmarshal(data, &request) != nil {
					t.Error("request JSON")
				}
				if request["model"] != "chosen-model" || request["stream"] != false || request["tools"] != nil {
					t.Error("request contract")
				}
				if provider == "anthropic" && request["output_config"] == nil || provider != "anthropic" && request["response_format"] == nil {
					t.Error("missing schema")
				}
				if provider == "anthropic" {
					wire, _ := json.Marshal(request["output_config"])
					for _, unsupported := range []string{"maxLength", "maxItems", "minimum"} {
						if bytes.Contains(wire, []byte(unsupported)) {
							t.Error("unsupported Anthropic schema constraint")
						}
					}
				}
				if !strings.Contains(string(data), "Untrusted bundle.json") || !strings.Contains(string(data), "PKGBUILD") {
					t.Error("lost bundle")
				}
				if !strings.Contains(string(data), "Custom shared audit prompt") {
					t.Error("configured prompt did not reach API")
				}
				io.WriteString(w, providerEnvelope(provider, providerTestReport))
			}))
			defer s.Close()
			c := auditConfig{Provider: provider, Model: "chosen-model", BaseURL: s.URL + "/v1", APIKeyEnv: "TEST_KEY"}
			prompt := "Custom shared audit prompt"
			c.Prompt = &prompt
			bundle := auditBundle{Identity: recipeIdentity{Pkgbase: "fixture"}, Files: []recipeFile{{Path: "PKGBUILD", Text: "pkgname=fixture\n"}}}
			report, err := auditHTTP(bundle, c, runConfig{}.withDefaults(), s.Client().Transport)
			if err != nil || report.Summary != "recipe reviewed" || calls.Load() != 1 {
				t.Fatalf("report=%#v err=%v calls=%d", report, err, calls.Load())
			}
		})
	}
}

func TestProviderHTTPRejectsFailureWithoutRetryOrSecretDisclosure(t *testing.T) {
	for _, provider := range []string{"anthropic", "openai-compatible"} {
		for _, failure := range []string{"401", "429", "500", "redirect", "malformed", "truncated", "action", "missing", "duplicate", "reflected-secret", "oversized", "path", "tool", "utf8"} {
			t.Run(provider+"/"+failure, func(t *testing.T) {
				t.Setenv("TEST_KEY", "credential-fixture")
				var calls atomic.Int32
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					report := providerTestReport
					switch failure {
					case "401":
						w.WriteHeader(401)
						io.WriteString(w, "credential-fixture")
						return
					case "429":
						w.WriteHeader(429)
						return
					case "500":
						w.WriteHeader(500)
						return
					case "redirect":
						w.Header().Set("Location", "/leak")
						w.WriteHeader(307)
						return
					case "malformed":
						io.WriteString(w, "credential-fixture")
						return
					case "action":
						report = strings.TrimSuffix(report, "}") + `,"action":"allow"}`
					case "missing":
						report = `{"summary":"x","risk":"low"}`
					case "duplicate":
						report = strings.TrimSuffix(report, "}") + `,"risk":"high"}`
					case "reflected-secret":
						report = strings.ReplaceAll(report, "recipe reviewed", "credential-fixture")
					case "oversized":
						io.WriteString(w, strings.Repeat(" ", maxProviderEnvelopeBytes+1))
						return
					case "path":
						report = strings.ReplaceAll(report, "PKGBUILD", "../PKGBUILD")
					case "utf8":
						w.Write([]byte{0xff})
						return
					}
					envelope := providerEnvelope(provider, report)
					if failure == "truncated" {
						envelope = strings.ReplaceAll(strings.ReplaceAll(envelope, `"end_turn"`, `"max_tokens"`), `"stop"`, `"length"`)
					}
					if failure == "tool" {
						envelope = strings.ReplaceAll(strings.ReplaceAll(envelope, `"end_turn"`, `"tool_use"`), `"stop"`, `"tool_calls"`)
					}
					io.WriteString(w, envelope)
				}))
				defer s.Close()
				_, err := auditHTTP(auditBundle{}, auditConfig{Provider: provider, Model: "m", BaseURL: s.URL, APIKeyEnv: "TEST_KEY"}, runConfig{}.withDefaults(), s.Client().Transport)
				if err == nil || strings.Contains(err.Error(), "credential-fixture") || calls.Load() != 1 {
					t.Fatalf("err=%v calls=%d", err, calls.Load())
				}
			})
		}
	}
}

func TestProviderHTTPCancellation(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "signal"}[interrupt], func(t *testing.T) {
			t.Setenv("TEST_KEY", "credential-fixture")
			ready := make(chan struct{})
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(ready)
				<-r.Context().Done()
			}))
			defer s.Close()
			signals := make(chan os.Signal, 1)
			config := runConfig{codexTimeout: time.Second, signals: signals}.withDefaults()
			if interrupt {
				go func() { <-ready; signals <- syscall.SIGTERM }()
			}
			start := time.Now()
			_, err := auditHTTP(auditBundle{}, auditConfig{Provider: "openai", Model: "m", BaseURL: s.URL, APIKeyEnv: "TEST_KEY"}, config, s.Client().Transport)
			if err == nil || time.Since(start) > 3*time.Second {
				t.Fatalf("cancellation failed: %v", err)
			}
		})
	}
}

func TestProviderClaudeContract(t *testing.T) {
	dir := t.TempDir()
	argsPath, inputPath, cwdPath := filepath.Join(dir, "args"), filepath.Join(dir, "input"), filepath.Join(dir, "cwd")
	t.Setenv("ARGS_FILE", argsPath)
	t.Setenv("INPUT_FILE", inputPath)
	t.Setenv("CWD_FILE", cwdPath)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	writeExecutable(t, dir, "claude", `#!/bin/sh
if [ "$1" = --version ]; then printf '2.1.325 (Claude Code)\n'; exit 0; fi
printf '%s\n' "$@" > "$ARGS_FILE"
pwd > "$CWD_FILE"
cat > "$INPUT_FILE"
printf '%s' "$CLAUDE_OUTPUT"
`)
	output, _ := json.Marshal(claudeTestEnvelope())
	t.Setenv("CLAUDE_OUTPUT", string(output))
	prompt := "Custom shared audit prompt"
	_, err := auditClaude(auditBundle{}, auditConfig{Model: "selected-model", Prompt: &prompt}, runConfig{}.withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	args := readString(t, argsPath)
	if !strings.Contains(args, "--system-prompt\n"+prompt) {
		t.Fatal("configured prompt did not reach Claude")
	}
	for _, pair := range []string{"--tools\n\n", "--strict-mcp-config\n", "--mcp-config\n{\"mcpServers\":{}}", "--setting-sources\n\n", "--settings\n{\"disableAllHooks\":true}", "--model\nselected-model", "--no-session-persistence", "--disable-slash-commands", "--permission-mode\ndontAsk"} {
		if !strings.Contains(args, pair) {
			t.Errorf("missing %q", pair)
		}
	}
	if !strings.HasPrefix(readString(t, inputPath), "Untrusted bundle.json:\n") {
		t.Fatal("stdin bundle missing")
	}
	if _, err := os.Stat(strings.TrimSpace(readString(t, cwdPath))); !os.IsNotExist(err) {
		t.Fatal("private attempt directory retained")
	}
	other := t.TempDir()
	writeExecutable(t, other, "claude-custom", "#!/bin/sh\nif [ \"$1\" = --version ]; then echo '2.1.325 (Claude Code)'; exit 0; fi\ncat >/dev/null\n: > \"$ARGS_FILE.custom\"\nprintf '%s' \"$CLAUDE_OUTPUT\"\n")
	t.Setenv("AUROSCOPE_CLAUDE", filepath.Join(other, "claude-custom"))
	if _, err := auditClaude(auditBundle{}, auditConfig{}, runConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(argsPath + ".custom"); err != nil {
		t.Fatal("AUROSCOPE_CLAUDE did not select the executable")
	}
	t.Setenv("AUROSCOPE_CLAUDE", "")
	if !strings.Contains(args, "--json-schema\n"+auditOutputSchema+"\n") {
		t.Error("audit schema was not passed as structured output")
	}
	// A schema-valid report in the human text result alone is not an audit:
	// only the structured output object is accepted.
	textOnly, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": providerTestReport})
	for _, output := range []string{
		`{"type":"result","subtype":"error","is_error":true,"result":"credential-fixture"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"not json"}`,
		string(textOnly),
		`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":null}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":"` + strings.ReplaceAll(providerTestReport, `"`, `\"`) + `"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":{"summary":"x","risk":"safe","findings":[],"uncertainty":"","inspect":[]}}`,
	} {
		t.Setenv("CLAUDE_OUTPUT", output)
		_, err := auditClaude(auditBundle{}, auditConfig{}, runConfig{}.withDefaults())
		if err == nil || strings.Contains(err.Error(), "credential-fixture") {
			t.Fatalf("error=%v", err)
		}
	}
}

// Runs both in the checkout and against the installed Arch artifact. No real
// package is built: fake Paru's final handoff only records the approved targets.
func TestProviderSelectionAndNoBypass(t *testing.T) {
	for _, choice := range []string{"approve", "skip", "cancel", "retry"} {
		t.Run(choice, func(t *testing.T) {
			dir := t.TempDir()
			writeProviderConfig(t, `{"provider":"claude-code"}`)
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
			calls := filepath.Join(dir, "calls")
			codexMarker := filepath.Join(dir, "codex-called")
			claudeCalls := filepath.Join(dir, "claude-calls")
			t.Setenv("CODEX_MARKER", codexMarker)
			t.Setenv("CLAUDE_CALLS", claudeCalls)
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			writeExecutable(t, dir, "claude", `#!/bin/sh
if [ "$1" = --version ]; then printf '2.1.325 (Claude Code)\n'; exit 0; fi
cat >/dev/null
printf 'audit\n' >> "$CLAUDE_CALLS"
if [ "$CLAUDE_FAIL" = 1 ]; then printf 'private-diagnostic' >&2; exit 3; fi
printf '%s' "$CLAUDE_OUTPUT"
`)
			output, _ := json.Marshal(claudeTestEnvelope())
			t.Setenv("CLAUDE_OUTPUT", string(output))
			input := "approve\n"
			wantCalls, wantStatus := 1, 0
			if choice != "approve" {
				t.Setenv("CLAUDE_FAIL", "1")
				input = choice + "\n"
				if choice == "cancel" {
					wantStatus = 1
				}
				if choice == "retry" {
					input = "retry\nskip\n"
					wantCalls = 2
				}
			}
			config := runConfig{
				paruPath:  fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n"),
				codexPath: writeExecutable(t, dir, "codex", "#!/bin/sh\n: > \"$CODEX_MARKER\"\nexit 99\n"),
				statePath: filepath.Join(dir, "state.sqlite3"), cloneDir: filepath.Join(dir, "clones"),
				stdin: strings.NewReader(input),
			}
			var out bytes.Buffer
			config.stdout, config.stderr = &out, &out
			status := 0
			if binary := os.Getenv("AUROSCOPE_TEST_BINARY"); binary != "" {
				cmd := exec.Command(binary, "-S", "hello")
				cmd.Env = append(os.Environ(), "AUROSCOPE_PARU="+config.paruPath, "AUROSCOPE_CODEX="+config.codexPath, "AUROSCOPE_STATE="+config.statePath, "AUROSCOPE_CLONE_DIR="+config.cloneDir)
				cmd.Stdin = config.stdin
				cmd.Stdout = &out
				cmd.Stderr = &out
				status = commandStatus(cmd.Run())
			} else {
				status = run([]string{"-S", "hello"}, config)
			}
			if status != wantStatus {
				t.Fatalf("status=%d output=%s", status, out.String())
			}
			if strings.Count(readString(t, claudeCalls), "audit\n") != wantCalls {
				t.Fatal("attempt count")
			}
			if _, err := os.Stat(codexMarker); !os.IsNotExist(err) {
				t.Fatal("Codex fallback invoked")
			}
			if strings.Contains(readString(t, calls), "--skipreview") != (choice == "approve") {
				t.Fatal("unexpected final build decision")
			}
			if strings.Contains(out.String(), "private-diagnostic") {
				t.Fatal("diagnostic leaked")
			}
		})
	}
}

func TestProviderInvalidConfigDoesNotAffectOfficialOperations(t *testing.T) {
	writeProviderConfig(t, `{"api_key":"private-value"}`)
	dir := t.TempDir()
	path := writeExecutable(t, dir, "paru", "#!/bin/sh\nprintf native\nexit 17\n")
	for _, args := range [][]string{{"--version"}, {"-S", "--repo", "tree"}} {
		var out bytes.Buffer
		if status := run(args, runConfig{paruPath: path, stdin: strings.NewReader(""), stdout: &out, stderr: &out}); status != 17 || out.String() != "native" {
			t.Fatalf("status=%d output=%s", status, out.String())
		}
	}
}
