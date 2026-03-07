package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"despatch/internal/models"
)

func TestConsumePasswordResetTokenSingleUseUnderRace(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "race-reset@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tokenHash := "race-token-hash"
	if _, err := st.CreatePasswordResetToken(ctx, u.ID, tokenHash, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	var mu sync.Mutex
	successes := 0
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, consumeErr := st.ConsumePasswordResetToken(ctx, tokenHash)
			if consumeErr == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one successful token consumption, got %d", successes)
	}
}

func TestReservePasswordResetTokenSingleLeaseUnderRace(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "reserve-reset@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tokenHash := "reserve-token-hash"
	if _, err := st.CreatePasswordResetToken(ctx, u.ID, tokenHash, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	var mu sync.Mutex
	successes := 0
	for i := 0; i < workers; i++ {
		reservationID := fmt.Sprintf("reservation-%d", i)
		go func(id string) {
			defer wg.Done()
			_, reserveErr := st.ReservePasswordResetToken(ctx, tokenHash, id, time.Minute)
			if reserveErr == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(reservationID)
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one successful reservation, got %d", successes)
	}
}

func TestReleasePasswordResetTokenReservationMakesTokenReusable(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "release-reset@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tokenHash := "release-token-hash"
	if _, err := st.CreatePasswordResetToken(ctx, u.ID, tokenHash, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	if _, err := st.ReservePasswordResetToken(ctx, tokenHash, "lease-a", time.Minute); err != nil {
		t.Fatalf("reserve token: %v", err)
	}
	if err := st.ReleasePasswordResetTokenReservation(ctx, tokenHash, "lease-a"); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
	token, err := st.ReservePasswordResetToken(ctx, tokenHash, "lease-b", time.Minute)
	if err != nil {
		t.Fatalf("reserve token second time: %v", err)
	}
	if token.ReservationID == nil || *token.ReservationID != "lease-b" {
		t.Fatalf("expected reservation lease-b, got %+v", token.ReservationID)
	}
}

func TestConsumeReservedPasswordResetTokenRequiresMatchingReservation(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "consume-reserved@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tokenHash := "consume-reserved-token-hash"
	if _, err := st.CreatePasswordResetToken(ctx, u.ID, tokenHash, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	if _, err := st.ReservePasswordResetToken(ctx, tokenHash, "lease-a", time.Minute); err != nil {
		t.Fatalf("reserve token: %v", err)
	}
	if _, err := st.ConsumeReservedPasswordResetToken(ctx, tokenHash, "lease-b"); err == nil {
		t.Fatalf("expected consume with wrong reservation to fail")
	}
	token, err := st.ConsumeReservedPasswordResetToken(ctx, tokenHash, "lease-a")
	if err != nil {
		t.Fatalf("consume with matching reservation: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("expected used_at to be set after reserved consume")
	}
	if token.ReservationID != nil || token.ReservedAt != nil {
		t.Fatalf("expected reservation fields to be cleared, got %+v %+v", token.ReservationID, token.ReservedAt)
	}
}
