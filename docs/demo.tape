# Generates docs/demo.gif.
# Run from the repository root: vhs docs/demo.tape

Output docs/demo.gif

Set FontSize 16
Set Width 1200
Set Height 640
Set Padding 20
Set TypingSpeed 40ms

# Build the binary off-camera.
Hide
Type "go build -o bin/stackrun ./cmd/stackrun && clear"
Enter
Sleep 4s
Show

Type "./bin/stackrun up -c examples/demo.yaml"
Sleep 500ms
Enter
Sleep 9s

# Graceful shutdown.
Ctrl+C
Sleep 3s