package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

var (
	errNoInstances = errors.New("no instances yet — create one with `llmaker up`")
	errNoRunning   = errors.New("no running instances — start one with `llmaker start <name>`")
)

func ambiguousErr(names []string) error {
	return fmt.Errorf("multiple instances running (%s) — choose one with --on <name>", strings.Join(names, ", "))
}

// healthLevel maps an instance health to a UI severity color.
func healthLevel(h engine.Health) ui.Level {
	switch h {
	case engine.HealthHealthy:
		return ui.LevelOK
	case engine.HealthStarting:
		return ui.LevelWarn
	case engine.HealthUnhealthy:
		return ui.LevelError
	default:
		return ui.LevelMuted
	}
}

// stateLevel maps a container state to a UI severity color.
func stateLevel(s engine.State) ui.Level {
	switch s {
	case engine.StateRunning:
		return ui.LevelOK
	case engine.StatePaused:
		return ui.LevelWarn
	case engine.StateExited:
		return ui.LevelError
	default:
		return ui.LevelMuted
	}
}

// healthLabel is a short human word for a health value.
func healthLabel(h engine.Health) string {
	if h == "" {
		return string(engine.HealthUnknown)
	}
	return string(h)
}

// resolveTarget picks the instance a model/chat command should act on. When a
// name is given it must exist; otherwise we default to the sole running
// instance, and refuse (with guidance) when the choice is ambiguous.
func (a *App) resolveTarget(ctx context.Context, rt engine.Runtime, name string) (engine.Instance, error) {
	if name != "" {
		return a.mustGet(ctx, rt, name)
	}
	ins, err := rt.List(ctx)
	if err != nil {
		return engine.Instance{}, err
	}
	var running []engine.Instance
	for _, in := range ins {
		if in.IsRunning() {
			running = append(running, in)
		}
	}
	switch len(running) {
	case 1:
		return running[0], nil
	case 0:
		if len(ins) == 0 {
			return engine.Instance{}, errNoInstances
		}
		return engine.Instance{}, errNoRunning
	default:
		names := make([]string, len(running))
		for i, in := range running {
			names[i] = in.Name
		}
		return engine.Instance{}, ambiguousErr(names)
	}
}

// listFleet returns the managed instances sorted by name. When withHealth is
// set, each running instance's facade is probed concurrently so `ls`/`top` can
// color health without serial latency.
func (a *App) listFleet(ctx context.Context, rt engine.Runtime, withHealth bool) ([]engine.Instance, error) {
	ins, err := rt.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(ins, func(i, j int) bool { return ins[i].Name < ins[j].Name })
	if withHealth {
		enrichHealth(ctx, a.Facade, ins)
	}
	return ins, nil
}

// enrichHealth probes each running instance's facade in parallel (bounded by a
// short timeout) and annotates Instance.Health in place.
func enrichHealth(ctx context.Context, fc facade.Client, ins []engine.Instance) {
	var wg sync.WaitGroup
	for i := range ins {
		if !ins[i].IsRunning() {
			ins[i].Health = engine.HealthUnhealthy
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := fc.Health(pctx, ins[i].URL()); err != nil {
				// Running but not answering yet — most likely still booting.
				ins[i].Health = engine.HealthStarting
			} else {
				ins[i].Health = engine.HealthHealthy
			}
		}(i)
	}
	wg.Wait()
}
