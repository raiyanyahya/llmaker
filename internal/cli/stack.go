package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// stackTemplate is a named, ready-to-apply stack.yaml scaffold.
type stackTemplate struct {
	name    string
	summary string
	content string
}

// stackTemplates are the starting points `llmaker stack init` can scaffold. Each
// is a complete stack — LLM + the services around it — wired by in-network name,
// so `apply` brings up a working system. The agent image must exist locally
// (`make image-agent`) or be pullable.
var stackTemplates = []stackTemplate{
	{
		name:    "rag",
		summary: "Doc Q&A: LLM + Qdrant + embeddings + a LangGraph RAG agent",
		content: `# RAG stack — ground answers in your own documents.
#   make image-agent                      # build the agent image once
#   llmaker apply -f stack.yaml           # bring the whole stack up
#   open the agent's URL (llmaker service ls) → ingest docs → ask
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the generation model  → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: qdrant               # vector database       → qdrant:6333
  - name: embeddings          # embeddings endpoint   → embeddings:80
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: agent                # LangGraph RAG app      → agent:8800
    # The agent's defaults already point at chat / qdrant / embeddings.
`,
	},
	{
		name:    "chatbot",
		summary: "Minimal assistant: LLM + agent (chat API + web UI), easy to extend",
		content: `# Chatbot stack — an LLM with a chat API and web UI.
#   make image-agent
#   llmaker apply -f stack.yaml
#
# The agent answers from the model alone here; add a vector DB + embeddings
# (` + "`llmaker service add qdrant && llmaker service add embeddings`" + `) and the
# agent automatically grounds answers in any docs you ingest.
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the chat model         → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: agent                # chat API + web UI      → agent:8800
`,
	},
	{
		name:    "faq",
		summary: "FAQ bot: LLM + Qdrant + embeddings + agent (short-answer tuned)",
		content: `# FAQ stack — answer questions from a knowledge base, concisely.
#   make image-agent
#   llmaker apply -f stack.yaml
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the answering model    → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: qdrant               # FAQ knowledge base     → qdrant:6333
  - name: embeddings
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: agent                # retrieval + answering  → agent:8800
    # Smaller chunks suit short FAQ entries.
    env: { CHUNK_SIZE: "500", CHUNK_OVERLAP: "80", TOP_K: "3" }
`,
	},
}

func findStackTemplate(name string) (stackTemplate, bool) {
	for _, t := range stackTemplates {
		if t.name == name {
			return t, true
		}
	}
	return stackTemplate{}, false
}

func stackTemplateNames() []string {
	names := make([]string, 0, len(stackTemplates))
	for _, t := range stackTemplates {
		names = append(names, t.name)
	}
	sort.Strings(names)
	return names
}

func newStackCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stack",
		Short:   "Scaffold whole-stack templates (RAG, chatbot, FAQ)",
		GroupID: groupFleet,
		Long: "Scaffold a ready-to-apply stack.yaml that wires an LLM together with the\n" +
			"services around it (vector DB, cache, embeddings, a LangGraph agent), so\n" +
			"`llmaker apply` brings up a working system in one step.",
	}
	cmd.AddCommand(newStackInitCmd(app))
	return cmd
}

func newStackInitCmd(app *App) *cobra.Command {
	var out string
	var force bool
	cmd := &cobra.Command{
		Use:       "init <template>",
		Short:     "Write a stack.yaml from a template",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: stackTemplateNames(),
		Long: "Write a stack.yaml from a template, then `llmaker apply` it.\n\n" +
			"Templates:\n" + stackTemplateHelp(),
		Example: "  llmaker stack init rag\n" +
			"  llmaker stack init chatbot -o chatbot.yaml\n" +
			"  make image-agent && llmaker apply -f stack.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			io := app.IO
			t := io.Theme
			if len(args) == 0 {
				io.Println(t.Heading("Stack templates"))
				io.Println(stackTemplateHelp())
				io.Println(t.Muted.Render("Scaffold one with ") + t.Accent.Render("llmaker stack init <template>"))
				return nil
			}
			tpl, ok := findStackTemplate(args[0])
			if !ok {
				return fmt.Errorf("unknown template %q (available: %s)", args[0], strings.Join(stackTemplateNames(), ", "))
			}
			if out == "" {
				out = "stack.yaml"
			}
			if _, err := os.Stat(out); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite, or -o to pick another path)", out)
			}
			if err := os.WriteFile(out, []byte(tpl.content), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			io.Println(t.SuccessLine("Wrote " + t.Value.Render(out) + " (" + tpl.name + " stack)"))
			io.Println()
			io.Println(t.Muted.Render("Next:"))
			io.Println("  " + t.Accent.Render("make image-agent") + t.Muted.Render("              # build the RAG agent image once"))
			io.Println("  " + t.Accent.Render("llmaker apply -f "+out) + t.Muted.Render("    # bring the whole stack up"))
			io.Println("  " + t.Accent.Render("llmaker ls") + t.Muted.Render("                    # see instances + services"))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output path (default: stack.yaml)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing file")
	return cmd
}

func stackTemplateHelp() string {
	var b strings.Builder
	for _, t := range stackTemplates {
		fmt.Fprintf(&b, "  %-9s %s\n", t.name, t.summary)
	}
	return b.String()
}
