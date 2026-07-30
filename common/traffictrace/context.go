package traffictrace

import (
	"context"
	"net"
)

type OuterFlowObserver interface {
	ObserveOuterFlow(OuterFlowObservation)
}

type observerContextKey struct{}

func WithObserver(ctx context.Context, observer OuterFlowObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, observerContextKey{}, observer)
}

func ObserverFromContext(ctx context.Context) OuterFlowObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(observerContextKey{}).(OuterFlowObserver)
	return observer
}

func ObserveOuterFlow(ctx context.Context, network string, src, dst net.Addr, source string) {
	observer := ObserverFromContext(ctx)
	if observer == nil {
		return
	}
	observer.ObserveOuterFlow(OuterFlowObservation{
		OuterConnID: NewOuterConnID(),
		Flow:        NewFlowTupleFromAddrs(network, src, dst, "", source, "physical", false),
	})
}
