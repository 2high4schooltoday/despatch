package workers

import (
	"context"
	"log"
	"time"

	"despatch/internal/config"
	"despatch/internal/store"
	"despatch/internal/update"
)

func StartUpdateWorkers(ctx context.Context, cfg config.Config, st *store.Store) {
	mgr := update.NewManager(cfg)
	go runUpdateLoop(ctx, mgr, st)
}

func runUpdateLoop(ctx context.Context, mgr *update.Manager, st *store.Store) {
	if err := mgr.AutomaticTick(ctx, st); err != nil {
		log.Printf("update_auto tick_failed error=%v", err)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := mgr.AutomaticTick(ctx, st); err != nil {
				log.Printf("update_auto tick_failed error=%v", err)
			}
		}
	}
}
