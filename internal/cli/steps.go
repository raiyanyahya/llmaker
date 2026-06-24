package cli

// step runs fn while showing a spinner labeled label, then prints a success or
// failure line. It returns fn's error unchanged so callers can react.
func (a *App) step(label string, fn func() error) error {
	sp := a.IO.Theme.NewSpinner(a.IO.Out, label)
	sp.Start()
	if err := fn(); err != nil {
		sp.Stop(a.IO.Theme.FailLine(label + " — " + err.Error()))
		return err
	}
	sp.Stop(a.IO.Theme.SuccessLine(label))
	return nil
}

// stepf is step with a value-returning function.
func stepf[T any](a *App, label string, fn func() (T, error)) (T, error) {
	sp := a.IO.Theme.NewSpinner(a.IO.Out, label)
	sp.Start()
	v, err := fn()
	if err != nil {
		sp.Stop(a.IO.Theme.FailLine(label + " — " + err.Error()))
		return v, err
	}
	sp.Stop(a.IO.Theme.SuccessLine(label))
	return v, nil
}
