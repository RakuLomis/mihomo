package route

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/metacubex/mihomo/component/tracer"
)

type tracingInfo struct {
	Enabled        bool   `json:"enabled"`
	Output         string `json:"output"`
	LastError      string `json:"last_error,omitempty"`
	WriteErrors    uint64 `json:"write_errors"`
	ActiveSessions int64  `json:"active_sessions"`
}

type tracingPatch struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Output  *string `json:"output,omitempty"`
}

func experimentalRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/tracing", getTracing)
	r.Patch("/tracing", patchTracing)
	return r
}

func getTracing(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, currentTracingInfo())
}

func patchTracing(w http.ResponseWriter, r *http.Request) {
	var req tracingPatch
	if err := render.DecodeJSON(r.Body, &req); err != nil && !errors.Is(err, io.EOF) {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}
	if err := tracer.Configure(tracer.ConfigPatch{Enabled: req.Enabled, Output: req.Output}); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, newError(err.Error()))
		return
	}
	render.JSON(w, r, currentTracingInfo())
}

func currentTracingInfo() tracingInfo {
	status := tracer.GetStatus()
	return tracingInfo{
		Enabled: status.Enabled, Output: status.Output, LastError: status.LastError,
		WriteErrors: status.WriteErrors, ActiveSessions: status.ActiveSessions,
	}
}
