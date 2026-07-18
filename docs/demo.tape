# demo.tape — source for docs/demo.gif (the README hero recording).
#
# To re-record (requires vhs, ttyd, ffmpeg — https://github.com/charmbracelet/vhs):
#   1. make build image-ollama-cpu && export PATH="$PWD/bin:$PATH"
#   2. In an empty directory, write the stack.yaml below, then: vhs demo.tape
#   3. Copy the resulting demo.gif over docs/demo.gif
#
# stack.yaml used by the recording:
#   name: demo
#   instances:
#     - name: chat
#       model: qwen2.5:0.5b
#       image: ghcr.io/raiyanyahya/llmaker-ollama:cpu
#   services:
#     - use: qdrant                 # → qdrant:6333
#     - { name: cache, use: redis } # → cache:6379
#
# Tip: run the stack once before recording, then remove the containers with
# plain `docker rm -f` (keeping the named volumes) — the recorded apply then
# reuses the cached model instead of downloading it on camera.

Output demo.gif

Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 640
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Sleep 500ms

Type "llmaker apply -f stack.yaml"
Sleep 500ms
Enter
Wait+Screen@180s /Applied stack.yaml/
Sleep 2s

Type "llmaker ls"
Sleep 300ms
Enter
Sleep 3.5s

Type `llmaker chat chat -m "In one sentence: why self-host your LLM stack?"`
Sleep 300ms
Enter
Sleep 8s

Type "llmaker top"
Sleep 300ms
Enter
Sleep 7s
Type "q"
Sleep 1s
