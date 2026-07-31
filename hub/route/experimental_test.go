package route

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/component/tracer"
)

func resetTracing(t *testing.T) {
	t.Helper()
	enabled := false
	output := ""
	sessionID := ""
	if err := tracer.Configure(tracer.ConfigPatch{Enabled: &enabled, Output: &output, SessionID: &sessionID}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = tracer.Configure(tracer.ConfigPatch{Enabled: &enabled, Output: &output, SessionID: &sessionID})
	})
}

func tracingRequest(t *testing.T, handler http.HandlerFunc, method, body string) (*httptest.ResponseRecorder, tracingInfo) {
	t.Helper()
	req := httptest.NewRequest(method, "/experimental/tracing", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler(resp, req)
	var info tracingInfo
	if resp.Code < 400 {
		if err := json.Unmarshal(resp.Body.Bytes(), &info); err != nil {
			t.Fatalf("decode response %q: %v", resp.Body.String(), err)
		}
	}
	return resp, info
}

func TestGetTracingUsesStableStdoutRepresentation(t *testing.T) {
	resetTracing(t)
	resp, info := tracingRequest(t, getTracing, http.MethodGet, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if info.Enabled || info.Output != "" || info.WriteErrors != 0 || info.ActiveSessions != 0 {
		t.Fatalf("unexpected tracing info: %+v", info)
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["output"]; !ok {
		t.Fatal("output field must be present for stdout")
	}
}

func TestPatchTracingIsAtomic(t *testing.T) {
	resetTracing(t)
	validPath := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := tracer.Configure(tracer.ConfigPatch{Output: &validPath}); err != nil {
		t.Fatal(err)
	}

	invalidPath := filepath.Join(t.TempDir(), "missing", "trace.jsonl")
	body, err := json.Marshal(map[string]any{"enabled": true, "output": invalidPath})
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := tracingRequest(t, patchTracing, http.MethodPatch, string(body))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	status := tracer.GetStatus()
	if status.Enabled || status.Output != validPath {
		t.Fatalf("failed patch changed state: %+v", status)
	}
}

func TestPatchTracingEnablesStdoutAndAcceptsEmptyBody(t *testing.T) {
	resetTracing(t)
	resp, info := tracingRequest(t, patchTracing, http.MethodPatch, `{"enabled":true,"output":""}`)
	if resp.Code != http.StatusOK || !info.Enabled || info.Output != "" {
		t.Fatalf("status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}

	resp, info = tracingRequest(t, patchTracing, http.MethodPatch, "")
	if resp.Code != http.StatusOK || !info.Enabled {
		t.Fatalf("empty patch status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}
}

func TestPatchTracingRejectsMalformedJSON(t *testing.T) {
	resetTracing(t)
	resp, _ := tracingRequest(t, patchTracing, http.MethodPatch, "{")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestGetTracingCapabilities(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tracing/capabilities", nil)
	resp := httptest.NewRecorder()
	experimentalRouter().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var got tracer.Capabilities
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode capabilities response: %v", err)
	}
	if want := tracer.CurrentCapabilities(); !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %+v, want %+v", got, want)
	}
}

func TestTracingCapabilitiesRejectsUnsupportedMethods(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/tracing/capabilities", nil)
			resp := httptest.NewRecorder()
			experimentalRouter().ServeHTTP(resp, req)
			if resp.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestPatchTracingSessionIDSetOmitAndClear(t *testing.T) {
	resetTracing(t)
	resp, info := tracingRequest(t, patchTracing, http.MethodPatch, `{"session_id":"session-one"}`)
	if resp.Code != http.StatusOK || info.SessionID != "session-one" {
		t.Fatalf("set session status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}

	resp, info = tracingRequest(t, patchTracing, http.MethodPatch, `{"enabled":false}`)
	if resp.Code != http.StatusOK || info.SessionID != "session-one" {
		t.Fatalf("omitted session status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}

	resp, info = tracingRequest(t, patchTracing, http.MethodPatch, `{"session_id":""}`)
	if resp.Code != http.StatusOK || info.SessionID != "" {
		t.Fatalf("clear session status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}
}

func TestLegacyTracingPayloadRemainsCompatible(t *testing.T) {
	resetTracing(t)
	resp, info := tracingRequest(t, patchTracing, http.MethodPatch, `{"enabled":true,"output":""}`)
	if resp.Code != http.StatusOK || !info.Enabled || info.Output != "" || info.SessionID != "" {
		t.Fatalf("legacy patch status=%d info=%+v body=%s", resp.Code, info, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if value, ok := raw["session_id"]; !ok || value != "" {
		t.Fatalf("session_id must be explicit in tracing state: %v", raw)
	}
}
