package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

func newChatCmd(app *App) *cobra.Command {
	var model, message, on string
	cmd := &cobra.Command{
		Use:     "chat [name]",
		Short:   "Chat with an instance to sanity-check it",
		GroupID: groupFleet,
		Args:    cobra.MaximumNArgs(1),
		Long: `Open an interactive chat session with an instance (type /exit or Ctrl-D to quit),
send a single prompt with --message, or pipe a prompt via stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := on
			if len(args) > 0 {
				name = args[0]
			}
			return runChat(cmd.Context(), app, chatOptions{name: name, model: model, message: message})
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model to chat with (defaults to the instance default)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "send a single message and exit")
	cmd.Flags().StringVar(&on, "on", "", "target instance (alternative to the positional name)")
	return cmd
}

type chatOptions struct {
	name    string
	model   string
	message string
}

func runChat(ctx context.Context, app *App, opts chatOptions) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	in, err := app.resolveTarget(ctx, rt, opts.name)
	if err != nil {
		return err
	}
	if err := requireRunning(in); err != nil {
		return err
	}

	model := opts.model
	if model == "" {
		model = defaultModelFor(ctx, app, in.URL(), in.Model)
	}
	if model == "" {
		return fmt.Errorf("no model available on %q — pull one with `llmaker pull <model> --on %s`", in.Name, in.Name)
	}

	// One-shot: explicit --message, or a prompt piped on stdin.
	if opts.message != "" {
		return chatOnce(ctx, app, in.URL(), model, opts.message)
	}
	if !app.IO.IsInteractive() {
		piped, _ := io.ReadAll(app.IO.In)
		prompt := strings.TrimSpace(string(piped))
		if prompt == "" {
			return fmt.Errorf("no prompt provided")
		}
		return chatOnce(ctx, app, in.URL(), model, prompt)
	}
	return chatInteractive(ctx, app, in.URL(), in.Name, model)
}

func chatOnce(ctx context.Context, app *App, baseURL, model, prompt string) error {
	req := facade.ChatRequest{Model: model, Messages: []facade.ChatMessage{{Role: "user", Content: prompt}}}
	err := app.Facade.Chat(ctx, baseURL, req, func(delta string) {
		app.IO.Printf("%s", delta)
	})
	app.IO.Println()
	return err
}

func chatInteractive(ctx context.Context, app *App, baseURL, name, model string) error {
	io := app.IO
	t := io.Theme

	io.Println(t.Heading("chat") + "  " + t.Muted.Render(name+" · "+model))
	io.Println(t.Muted.Render("Type a message and press enter. /exit or Ctrl-D to quit."))
	io.Println()

	history := []facade.ChatMessage{}
	scanner := bufio.NewScanner(io.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	userPrompt := t.Accent.Render("you ❯ ")
	for {
		io.Printf("%s", userPrompt)
		if !scanner.Scan() {
			io.Println()
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		switch line {
		case "":
			continue
		case "/exit", "/quit", "/q":
			return nil
		case "/reset", "/clear":
			history = history[:0]
			io.Println(t.Muted.Render("(conversation cleared)"))
			continue
		}

		history = append(history, facade.ChatMessage{Role: "user", Content: line})
		io.Printf("%s", t.Success.Render(ui.Truncate(model, 16)+" ❯ "))

		var reply strings.Builder
		err := app.Facade.Chat(ctx, baseURL, facade.ChatRequest{Model: model, Messages: history}, func(delta string) {
			reply.WriteString(delta)
			io.Printf("%s", delta)
		})
		io.Println()
		if err != nil {
			io.Println(t.FailLine(err.Error()))
			// Drop the unanswered user turn so history stays consistent.
			history = history[:len(history)-1]
			continue
		}
		history = append(history, facade.ChatMessage{Role: "assistant", Content: reply.String()})
	}
}

// defaultModelFor asks the facade for the instance default model, falling back
// to the label-recorded model when the facade can't be reached.
func defaultModelFor(ctx context.Context, app *App, baseURL, fallback string) string {
	if st, err := app.Facade.Status(ctx, baseURL); err == nil {
		if st.Models.Default != "" {
			return st.Models.Default
		}
		if len(st.Models.Installed) > 0 {
			return st.Models.Installed[0].Name
		}
	}
	return fallback
}
