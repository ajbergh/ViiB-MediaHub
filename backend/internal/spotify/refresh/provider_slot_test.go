package refresh

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackgroundAdmissionReservesInteractiveCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	bg := BackgroundContext(ctx)
	first, err := AcquireProviderSlot(bg)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := AcquireProviderSlot(bg)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	interactive, err := AcquireProviderSlot(ctx)
	if err != nil {
		t.Fatalf("reserved interactive capacity: %v", err)
	}
	interactive()
	blocked, stop := context.WithCancel(bg)
	stop()
	if release, err := AcquireProviderSlot(blocked); !errors.Is(err, context.Canceled) {
		if release != nil {
			release()
		}
		t.Fatalf("third background admission: %v", err)
	}
	first()
	first() // Release is idempotent.
	replacement, err := AcquireProviderSlot(bg)
	if err != nil {
		t.Fatalf("released background slot: %v", err)
	}
	replacement()
}
