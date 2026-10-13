package refresh

import (
	"context"
	"sync"
)

type backgroundKey struct{}

// BackgroundContext marks preparation work for bounded background admission.
// Interactive callers retain a dedicated slot even when preparation is busy.
func BackgroundContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

var backgroundProviderGate = make(chan struct{}, 2)
var totalProviderGate = make(chan struct{}, 3)

// AcquireProviderSlot shares admission across analysis and extension resources.
// Interactive requests remain serialized across session lifetimes. Background
// work has two slots and cannot consume the third, reserved interactive slot.
func AcquireProviderSlot(ctx context.Context) (func(), error) {
	lane := providerGate
	if background, _ := ctx.Value(backgroundKey{}).(bool); background {
		lane = backgroundProviderGate
	}
	select {
	case lane <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case totalProviderGate <- struct{}{}:
	case <-ctx.Done():
		<-lane
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-totalProviderGate
		<-lane
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { <-totalProviderGate; <-lane }) }, nil
}
