package route

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/metacubex/mihomo/component/tracer"
)

type tracingInfo struct {
	Enabled bool   `json:"enabled"`
	Output  string `json:"output,omitempty"`
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
	render.JSON(w, r, tracingInfo{
		Enabled: tracer.IsEnabled(),
		Output:  tracer.Output(),
	})
}

func patchTracing(w http.ResponseWriter, r *http.Request) {
	var req tracingPatch
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}
	if req.Enabled != nil {
		tracer.SetEnabled(*req.Enabled)
	}
	if req.Output != nil {
		if err := tracer.SetOutput(*req.Output); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, newError(err.Error()))
			return
		}
	}
	render.JSON(w, r, tracingInfo{
		Enabled: tracer.IsEnabled(),
		Output:  tracer.Output(),
	})
}
