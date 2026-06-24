// Package facadetest provides an in-memory facade.Client for testing CLI
// command logic without a real container or network.
package facadetest

import (
	"context"
	"fmt"
	"sync"

	"github.com/raiyanyahya/llmaker/internal/facade"
)

// Fake is a programmable facade.Client. Fields let a test set canned responses;
// the Calls slice records what was invoked for assertions.
type Fake struct {
	mu sync.Mutex

	HealthErr  error
	StatusResp *facade.Status
	StatusErr  error
	Models     *facade.ModelList
	PullEvents []facade.PullEvent
	PullErr    error
	ChatDeltas []string
	ChatErr    error

	Calls []string
}

func (f *Fake) record(name string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, name)
	f.mu.Unlock()
}

// CallCount returns how many times a named method was recorded.
func (f *Fake) CallCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if c == name {
			n++
		}
	}
	return n
}

func (f *Fake) Health(ctx context.Context, base string) (facade.Health, error) {
	f.record("Health")
	if f.HealthErr != nil {
		return facade.Health{}, f.HealthErr
	}
	return facade.Health{Status: "ok", Ready: true}, nil
}

func (f *Fake) Status(ctx context.Context, base string) (*facade.Status, error) {
	f.record("Status")
	if f.StatusErr != nil {
		return nil, f.StatusErr
	}
	if f.StatusResp != nil {
		return f.StatusResp, nil
	}
	return &facade.Status{}, nil
}

func (f *Fake) ListModels(ctx context.Context, base string) (*facade.ModelList, error) {
	f.record("ListModels")
	if f.Models != nil {
		return f.Models, nil
	}
	return &facade.ModelList{}, nil
}

func (f *Fake) Pull(ctx context.Context, base, model string, onProgress func(facade.PullEvent)) error {
	f.record("Pull:" + model)
	for _, e := range f.PullEvents {
		if onProgress != nil {
			onProgress(e)
		}
	}
	return f.PullErr
}

func (f *Fake) Delete(ctx context.Context, base, model string) error {
	f.record("Delete:" + model)
	return nil
}

func (f *Fake) SetDefault(ctx context.Context, base, model string) error {
	f.record("SetDefault:" + model)
	return nil
}

func (f *Fake) Chat(ctx context.Context, base string, req facade.ChatRequest, onDelta func(string)) error {
	f.record(fmt.Sprintf("Chat:%s", req.Model))
	for _, d := range f.ChatDeltas {
		if onDelta != nil {
			onDelta(d)
		}
	}
	return f.ChatErr
}

var _ facade.Client = (*Fake)(nil)
