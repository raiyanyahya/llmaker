package cli

import (
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		GroupID: groupAdvanced,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			io := app.IO
			t := io.Theme
			v := app.Version
			ver := v.Version
			if ver == "" {
				ver = "dev"
			}
			io.Println(t.Logo.Render("llmaker") + " " + t.Value.Render(ver))
			rows := [][2]string{
				{"commit", orDash(v.Commit)},
				{"built", orDash(v.Date)},
				{"go", runtime.Version()},
				{"platform", runtime.GOOS + "/" + runtime.GOARCH},
			}
			io.Println(t.KeyValues(rows))
			return nil
		},
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
