package cli

import (
	"context"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// gcNetworks garbage-collects group networks left empty after removals,
// printing a muted line per network so the cleanup is visible. Best-effort by
// design: a failure (or a runtime without the capability) just leaves the
// network for the next removal to sweep.
func gcNetworks(ctx context.Context, app *App, rt engine.Runtime) {
	np, ok := rt.(engine.NetworkPruner)
	if !ok {
		return
	}
	removed, _ := np.PruneNetworks(ctx)
	for _, n := range removed {
		app.IO.Println(app.IO.Theme.Muted.Render("• removed empty network " + n))
	}
}
