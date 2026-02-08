package logic

import (
	"context"
	"module/internal/framework"
	"time"
)

func Start(api framework.ModuleAPI) {
	api.Info("Module Logic Started")

	var cancelActive context.CancelFunc
	var activeCtx context.Context

	// Template for refreshable logic
	runLogic := func(config map[string]any) {
		if cancelActive != nil {
			api.Info("Refreshing logic with new configuration...")
			cancelActive()
		}

		activeCtx, cancelActive = context.WithCancel(api.Context())
		
		go func(ctx context.Context, cfg map[string]any) {
			// YOUR BUNDLE LOGIC HERE
			// Use ctx.Done() to check if you should stop
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					api.Info("Module heartbeat...")
				}
			}
		}(activeCtx, config)
	}

	// 1. Initial Start
	cfg := api.GetModuleConfig()
	runLogic(cfg)

	// 2. Monitor for changes (The Framework's Adapter will update the in-memory config)
	ticker := time.NewTicker(2 * time.Second)
	lastUpdate := time.Now() // Simple heuristic or track a specific key
	
	for {
		select {
		case <-api.Context().Done():
			return
		case <-ticker.C:
			// You can implement deeper comparison here to decide when to refresh
			// For example: if cfg["apiKey"] != lastSeenKey { runLogic(newCfg) }
		}
	}
}
