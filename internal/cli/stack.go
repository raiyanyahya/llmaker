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
		name:    "assistant",
		summary: "Private ChatGPT: a local model + Open WebUI (no agent image to build)",
		content: `# Assistant stack — a private, ChatGPT-style assistant over a local model.
#   llmaker stack up assistant            # scaffold + bring it up in one command
#   open the Open WebUI URL (see ` + "`llmaker service ls`" + `) and start chatting
#
# Pure public images — there's no agent image to build. Open WebUI reaches the
# model over the llmaker network at http://chat:8080/v1.
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the model behind the UI  → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: open-webui           # ChatGPT-style web UI     → open-webui:8080
`,
	},
	{
		name:    "voice",
		summary: "Talk to a model: Open WebUI + self-hosted Whisper STT (no agent image)",
		content: `# Voice stack — talk to a local model. Speech-to-text runs in the browser via
# self-hosted Whisper, wired into Open WebUI. Pure public images, nothing to build.
#   llmaker stack up voice
#   open the Open WebUI URL (` + "`llmaker service ls`" + `), click the mic, and speak
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the model behind the UI   → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: whisper              # speech-to-text            → whisper:8000
  - name: open-webui          # ChatGPT-style UI w/ voice → open-webui:8080
    use: open-webui
    env:
      AUDIO_STT_ENGINE: openai
      AUDIO_STT_OPENAI_API_BASE_URL: http://whisper:8000/v1
      AUDIO_STT_OPENAI_API_KEY: not-needed
`,
	},
	{
		name:    "rag",
		summary: "Doc Q&A: LLM + Qdrant + embeddings + RAG agent + Langfuse tracing",
		content: `# RAG stack — ground answers in your own documents, with tracing.
#   make image-agent                      # build the agent image once
#   llmaker apply -f stack.yaml           # bring the whole stack up
#   open the agent's URL (llmaker service ls) → ingest docs → ask
#   open Langfuse (langfuse's URL) to see each query traced
#     sign in: admin@llmaker.local / llmaker-dev
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
  - use: pgvector             # Postgres for Langfuse  → pgvector:5432
  - use: langfuse             # LLM observability      → langfuse:3000
  - use: agent                # LangGraph RAG app      → agent:8800
    # Defaults already point at chat / qdrant / embeddings; add tracing.
    env:
      LANGFUSE_HOST: http://langfuse:3000
      LANGFUSE_PUBLIC_KEY: pk-lf-llmaker
      LANGFUSE_SECRET_KEY: sk-lf-llmaker
`,
	},
	{
		name:    "research",
		summary: "Research assistant: LLM + web search (SearXNG) + Qdrant + embeddings + agent",
		content: `# Research stack — a tool-using assistant that searches the live web AND your
# own documents, then synthesizes an answer.
#   make image-agent
#   llmaker apply -f stack.yaml
#
# One-time step: enable JSON results in SearXNG so the agent can read them — in
# the mounted /etc/searxng/settings.yml set:
#     search:
#       formats: [html, json]
#   then: llmaker stop searxng && llmaker start searxng
#
# Ask the tool-using endpoint; the model picks web_search / knowledge_base / etc:
#   POST /api/agent  {"question": "What shipped in the latest Go release?"}
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # a tool-capable model    → chat:8080
    model: qwen2.5:7b         # web_search needs a model that calls tools reliably
    memory: 8g

services:
  - use: qdrant               # document store          → qdrant:6333
  - name: embeddings          # embeddings endpoint     → embeddings:80
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: searxng              # self-hosted web search  → searxng:8080
  - use: agent                # tool-using agent        → agent:8800
    env:
      SEARCH_URL: http://searxng:8080   # turns on the web_search tool
`,
	},
	{
		name:    "code",
		summary: "Code assistant: code LLM + Qdrant + embeddings + agent (Q&A over your repo)",
		content: `# Code assistant stack — question-answering and review over your own codebase.
#   make image-agent
#   llmaker apply -f stack.yaml
#
# Ingest source files, then ask grounded questions about them:
#   for f in $(git ls-files '*.go' '*.py'); do curl -F file=@$f $AGENT/api/ingest; done
#   curl $AGENT/api/chat -d '{"question":"where is auth handled?"}'
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # a code-tuned model     → chat:8080
    model: qwen2.5-coder:7b
    memory: 8g

services:
  - use: qdrant               # code chunks            → qdrant:6333
  - name: embeddings          # embeddings endpoint    → embeddings:80
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: agent                # retrieval + answering  → agent:8800
    # Larger chunks keep functions and context intact.
    env: { CHUNK_SIZE: "1200", CHUNK_OVERLAP: "200" }
`,
	},
	{
		name:    "chatbot",
		summary: "Multi-turn assistant with memory: LLM + Redis + agent (chat API + web UI)",
		content: `# Chatbot stack — an LLM with a chat API, web UI, and per-session memory.
#   make image-agent
#   llmaker apply -f stack.yaml
#
# Send a "session_id" with each /api/chat request and the agent remembers the
# conversation (kept in Redis). Add a vector DB + embeddings
# (` + "`llmaker service add qdrant && llmaker service add embeddings`" + `) and the
# agent automatically grounds answers in any docs you ingest.
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # the chat model         → chat:8080
    model: llama3:8b
    memory: 8g

services:
  - use: redis                # conversation memory    → redis:6379
  - use: agent                # chat API + web UI      → agent:8800
    env:
      REDIS_URL: redis://redis:6379   # persist per-session history (send session_id)
`,
	},
	{
		name:    "recommend",
		summary: "Recommendation engine: Qdrant + embeddings + agent (no LLM needed)",
		content: `# Recommendation stack — semantic "more like this" over your items.
#   make image-agent
#   llmaker apply -f stack.yaml
#
# No LLM instance: recommendations are pure vector similarity. Use the agent's
# API to load items and get recommendations:
#   POST /api/items      {"items":[{"id":"sku1","text":"...","metadata":{...}}]}
#   POST /api/recommend  {"query":"cozy winter jacket","k":5}
#   POST /api/recommend  {"like":["sku1","sku2"],"k":5}   # taste profile
version: "1"

services:
  - use: qdrant               # item vectors           → qdrant:6333
  - name: embeddings          # embeddings endpoint    → embeddings:80
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: agent                # /api/items + /api/recommend → agent:8800
    env: { EMBEDDINGS_URL: http://embeddings:80 }
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
	{
		name:    "sql",
		summary: "Talk to your database: LLM + Postgres + agent (read-only NL→SQL) + docs",
		content: `# SQL assistant stack — ask questions in plain English; the agent answers by
# running *read-only* SQL (enforced server-side) against Postgres, and can also
# ground answers in ingested docs.
#   make image-agent
#   llmaker stack up sql
#
# Load a schema into the bundled Postgres (or point SQL_DSN at your own database),
# then ask the tool-using endpoint:
#   POST /api/agent  {"question": "how many orders shipped last week?"}
version: "1"

defaults: { backend: ollama }

instances:
  - name: chat                # a tool-capable model    → chat:8080
    model: qwen2.5:7b         # the sql tool needs reliable tool-calling
    memory: 8g

services:
  - use: pgvector             # the SQL database        → pgvector:5432
  - use: qdrant               # document store          → qdrant:6333
  - name: embeddings
    use: embeddings
    env: { MODEL_ID: BAAI/bge-small-en-v1.5 }
  - use: agent                # tool-using agent + SQL  → agent:8800
    env:
      SQL_DSN: postgresql://llmaker:llmaker@pgvector:5432/llmaker
`,
	},
}

// stackContent returns the template body with a top-level `name:` ensured, so a
// scaffolded stack is automatically scoped for `apply --prune` (applying it
// never touches another stack's resources).
func stackContent(tpl stackTemplate) string {
	if strings.HasPrefix(tpl.content, "name:") || strings.Contains(tpl.content, "\nname:") {
		return tpl.content
	}
	marker := "version: \"1\"\n"
	if i := strings.Index(tpl.content, marker); i >= 0 {
		j := i + len(marker)
		return tpl.content[:j] + "name: " + tpl.name + "\n" + tpl.content[j:]
	}
	return "name: " + tpl.name + "\n" + tpl.content
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
		Short:   "Scaffold & run whole-stack templates (assistant, RAG, research, code, chatbot, FAQ, recommend)",
		GroupID: groupFleet,
		Long: "Scaffold a ready-to-apply stack.yaml that wires an LLM together with the\n" +
			"services around it (web UI, vector DB, cache, embeddings, a LangGraph agent),\n" +
			"so `llmaker apply` brings up a working system in one step. `stack up` does\n" +
			"the scaffold and the apply together.",
	}
	cmd.AddCommand(newStackInitCmd(app), newStackUpCmd(app))
	return cmd
}

func newStackUpCmd(app *App) *cobra.Command {
	var out string
	var force, prune, noPull bool
	cmd := &cobra.Command{
		Use:       "up <template>",
		Short:     "Scaffold a stack template and apply it in one command",
		Args:      cobra.ExactArgs(1),
		ValidArgs: stackTemplateNames(),
		Long: "Write a stack.yaml from a template and immediately `apply` it — the\n" +
			"fastest path from nothing to a running stack. If the stack file already\n" +
			"exists it is reused as-is (pass --force to regenerate from the template).\n\n" +
			"Templates:\n" + stackTemplateHelp(),
		Example: "  llmaker stack up assistant     # a private ChatGPT-style UI over a local model\n" +
			"  llmaker stack up rag           # full RAG stack (build the agent image first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tpl, ok := findStackTemplate(args[0])
			if !ok {
				return fmt.Errorf("unknown template %q (available: %s)", args[0], strings.Join(stackTemplateNames(), ", "))
			}
			if out == "" {
				out = "stack.yaml"
			}
			io := app.IO
			t := io.Theme
			if _, err := os.Stat(out); err == nil && !force {
				io.Println(t.Muted.Render("Using existing " + out + " (pass --force to regenerate from the template)."))
			} else {
				if err := os.WriteFile(out, []byte(stackContent(tpl)), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", out, err)
				}
				io.Println(t.SuccessLine("Wrote " + t.Value.Render(out) + " (" + tpl.name + " stack)"))
			}
			io.Println()
			return runApply(cmd.Context(), app, applyOptions{file: out, prune: prune, noPull: noPull})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&out, "out", "o", "", "stack file path (default: stack.yaml)")
	f.BoolVarP(&force, "force", "f", false, "regenerate the stack file from the template before applying")
	f.BoolVar(&prune, "prune", false, "remove managed resources not present in the stack")
	f.BoolVar(&noPull, "no-pull", false, "don't preload models")
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
			if err := os.WriteFile(out, []byte(stackContent(tpl)), 0o644); err != nil {
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
