package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderHTTPThinking(t *testing.T) {
	for _, provider := range []string{"openai", "openai-compatible", "anthropic"} {
		for _, effort := range []string{"", "medium", "high", `future\level"literal`} {
			t.Run(provider+"/"+effort, func(t *testing.T) {
				t.Setenv("TEST_KEY", "credential-fixture")
				calls := 0
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					got, present := payload["reasoning_effort"]
					if provider == "anthropic" {
						if present {
							t.Error("OpenAI effort leaked into Anthropic request")
						}
						output := payload["output_config"].(map[string]any)
						got, present = output["effort"]
						if output["format"] == nil {
							t.Error("lost structured output")
						}
					} else if payload["response_format"] == nil {
						t.Error("lost structured output")
					}
					if present != (effort != "") || present && got != effort {
						t.Errorf("effort=%v present=%v", got, present)
					}
					if strings.HasPrefix(effort, "future") {
						w.WriteHeader(400)
						return
					}
					io.WriteString(w, providerEnvelope(provider, providerTestReport))
				}))
				defer s.Close()
				c := auditConfig{Provider: provider, Model: "fixture", BaseURL: s.URL, APIKeyEnv: "TEST_KEY"}
				if effort != "" {
					c.Thinking = &effort
				}
				bundle := auditBundle{Files: []recipeFile{{Path: "PKGBUILD", Text: "pkgname=fixture\n"}}}
				_, err := auditHTTP(bundle, c, runConfig{}.withDefaults(), s.Client().Transport)
				if (err != nil) != strings.HasPrefix(effort, "future") || calls != 1 {
					t.Fatalf("err=%v calls=%d", err, calls)
				}
			})
		}
	}
}

// This path also runs against /usr/bin/auroscope in the installed-artifact gate.
func TestProviderClaudeThinking(t *testing.T) {
	binary := os.Getenv("AUROSCOPE_TEST_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "auroscope")
		if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/auroscope").CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	for _, effort := range []string{"", "medium", "high", `future\level"literal`} {
		t.Run(effort, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			t.Setenv("CAPTURE_EFFORT", filepath.Join(dir, "effort"))
			writeExecutable(t, dir, "claude", `#!/bin/sh
if [ "$1" = --version ]; then printf '2.1.325 (Claude Code)\n'; exit 0; fi
set -eu
value=''
count=0
while [ "$#" -gt 0 ]; do
 if [ "$1" = --effort ]; then shift; value=$1; count=$((count + 1)); fi
 shift
done
[ "$count" -le 1 ]
printf '%s' "$value" > "$CAPTURE_EFFORT"
case "$value" in future*) exit 2 ;; esac
printf '%s' '{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":{"summary":"reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}}'
`)
			config := auditConfig{Provider: "claude-code"}
			if effort != "" {
				config.Thinking = &effort
			}
			data, _ := json.Marshal(config)
			writeProviderConfig(t, string(data))
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
			calls := filepath.Join(dir, "calls")
			paru := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
			cmd := exec.Command(binary, "-S", "hello")
			cmd.Env = append(os.Environ(), "AUROSCOPE_PARU="+paru, "AUROSCOPE_CODEX=/does/not/exist", "AUROSCOPE_STATE="+filepath.Join(dir, "state"), "AUROSCOPE_CLONE_DIR="+filepath.Join(dir, "clones"))
			failure := strings.HasPrefix(effort, "future")
			input := "approve\n"
			if failure {
				input = "skip\n"
			}
			cmd.Stdin = strings.NewReader(input)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("audit: %v %s", err, out)
			}
			if got := readString(t, filepath.Join(dir, "effort")); got != effort {
				t.Fatalf("got %q want %q", got, effort)
			}
			if strings.Contains(readString(t, calls), "--skipreview") == failure {
				t.Fatal("incorrect final handoff")
			}
		})
	}
}
