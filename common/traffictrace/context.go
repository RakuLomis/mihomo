package traffictrace

import (
	"context"
	"net"
	"sync"
)

type OuterFlowObserver interface {
	ObserveOuterFlow(OuterFlowObservation)
}

const (
	CarrierRelationCreated = "created"
	CarrierRelationReused  = "reused"
)

const (
	CarrierLifecycleOpen       = "carrier_open"
	CarrierLifecyclePathUpdate = "carrier_path_update"
	CarrierLifecycleClose      = "carrier_close"
)

type CarrierLifecycleObservation struct {
	Type        string
	Observation OuterFlowObservation
}

type CarrierLifecycleObserver func(CarrierLifecycleObservation)

var carrierLifecycle struct {
	sync.RWMutex
	observer CarrierLifecycleObserver
}

// SetCarrierLifecycleObserver installs the process-level lifecycle consumer.
// The tracer owns this hook; transports remain independent from its package.
func SetCarrierLifecycleObserver(observer CarrierLifecycleObserver) {
	carrierLifecycle.Lock()
	carrierLifecycle.observer = observer
	carrierLifecycle.Unlock()
}

func NotifyCarrierLifecycle(event CarrierLifecycleObservation) {
	carrierLifecycle.RLock()
	observer := carrierLifecycle.observer
	carrierLifecycle.RUnlock()
	if observer == nil {
		return
	}
	event.Observation = event.Observation.Clone()
	observer(event)
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
	NotifyOuterFlow(ctx, OuterFlowObservation{
		OuterConnID: NewOuterConnID(),
		Flow:        NewFlowTupleFromAddrs(network, src, dst, "", source, "physical", false),
		Relation:    CarrierRelationCreated,
		Generation:  1,
	})
}

// NotifyOuterFlow publishes an existing observation without allocating a new
// outer connection identity. Multiplexed transports use this when a logical
// stream reuses a physical carrier created by an earlier stream.
func NotifyOuterFlow(ctx context.Context, observation OuterFlowObservation) {
	observer := ObserverFromContext(ctx)
	if observer == nil {
		return
	}
	observer.ObserveOuterFlow(observation.Clone())
}
