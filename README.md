# stackrun

[![CI](https://github.com/rithikamandiv-ux/stackrun/actions/workflows/ci.yml/badge.svg)](https://github.com/rithikamandiv-ux/stackrun/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/rithikamandiv-ux/stackrun)](go.mod)
[![Licence: MIT](https://img.shields.io/github/license/rithikamandiv-ux/stackrun)](LICENSE)

Run every service your project needs with one command, see all their output
in one place, and stop them all cleanly.

![stackrun starting three services, restarting a failing one, and shutting down](docs/demo.gif)

## Why stackrun?

A typical full-stack project needs several processes running during
development: a frontend dev server, a backend API, a worker, perhaps a
machine learning service. Running them usually means a terminal tab per
process, logs scattered across windows, and manually restarting anything
that crashes.

stackrun reads one small config file and runs everything for you:

- **One command** starts every service.
- **Merged output** with an aligned, colour-coded prefix per service.
- **Automatic restarts** with exponential backoff when a service crashes.
- **Graceful shutdown**: Ctrl+C stops every service and every process it
  started, leaving nothing running in the background.
- **Strict validation** that reports every config mistake at once.
- **A single binary** with no runtime dependencies.

## Installation

### Download a release

Prebuilt binaries for macOS and Linux (Intel and Apple Silicon) are
available on the [Releases](https://github.com/rithikamandiv-ux/stackrun/releases)
page. Download the archive for your platform, extract it, and move
`stackrun` somewhere on your `PATH`.

### With Go

```sh
go install github.com/rithikamandiv-ux/stackrun/cmd/stackrun@latest
```

This requires the Go version listed in [`go.mod`](go.mod) or newer.

## Quick start

Create a `stackrun.yaml` in your project root:

```yaml
services:
  api:
    command: npm run dev
    dir: ./server
    env:
      PORT: "4000"
    restart: on-failure

  web:
    command: npm run dev
    dir: ./client

  ml:
    command: uvicorn app:app --reload
    dir: ./ml-service
    stop_timeout: 30s
```

Check it, then start everything:

```sh
stackrun validate
stackrun up
```

Press **Ctrl+C** to stop all services. Press it a second time to stop them
immediately without waiting.

## Commands

| Command | Description |
|---|---|
| `stackrun up` | Start all services and stream their output |
| `stackrun validate` | Check the config file and report every problem |
| `stackrun version` | Print the version |

Every command accepts `-c` / `--config` to use a config file other than
`./stackrun.yaml`.

## Configuration

A config file contains a `services` map. Each key is a service name, used as
its output prefix. Names may contain letters, digits, `-` and `_`, and must
start with a letter or digit.

| Field | Required | Default | Description |
|---|---|---|---|
| `command` | Yes | | Shell command to run, through `/bin/sh -c`. Pipes, `&&` and variables work as in a terminal. |
| `dir` | No | The config file's folder | Working directory. Relative paths are resolved from the config file's folder, not from where you run stackrun. |
| `env` | No | | Extra environment variables. Services inherit your environment, and these values override it. Quote numbers, for example `PORT: "4000"`. |
| `restart` | No | `never` | When to restart: `never`, `on-failure` (non-zero exit or crash), or `always`. |
| `stop_timeout` | No | `10s` | How long to wait for a graceful stop before killing the service. Requires a unit, for example `500ms`, `30s`, `1m`. Maximum `5m`. |

Unknown fields are rejected, so a typo such as `comand` is reported instead
of being silently ignored.

## How it behaves

### Output

Each line is prefixed with its service name, padded so the separators line
up. Colours are used only when writing to a terminal, and are disabled when
`NO_COLOR` is set or `TERM=dumb`, so redirecting output to a file produces
clean text.

### Shutdown

On Ctrl+C (SIGINT) or SIGTERM, stackrun stops all services in parallel:

1. Sends **SIGTERM** to each service's **process group**, so the shell, the
   command, and anything it started all receive it.
2. Waits up to the service's `stop_timeout`.
3. Sends **SIGKILL** to the group if the service is still running.

A second Ctrl+C skips the wait and kills everything immediately.

### Restarts

With `on-failure` or `always`, a service that exits is restarted after a
delay that doubles each time: 1s, 2s, 4s, 8s, 16s, then 30s at most. After
5 consecutive restarts, stackrun gives up on that service and keeps the
others running. A run lasting at least 10 seconds counts as healthy and
resets the count.

Services are never restarted while stackrun is shutting down, and a service
that fails to start (for example, because its directory is missing) is not
retried, since retrying would fail the same way.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Every service exited successfully |
| `1` | A service failed, or the config is invalid |
| `130` | Stopped by Ctrl+C (SIGINT) |
| `143` | Stopped by SIGTERM |

These follow the Unix convention of 128 plus the signal number, so scripts
can tell an interruption apart from a failure.

## Security

stackrun runs the commands in your config file, with your permissions,
exactly as if you typed them into a terminal. This is the same trust model
as a Makefile or npm scripts.

**Only run `stackrun up` on config files you trust.** Running it in a
repository you downloaded from someone else runs their commands on your
machine. Read the `stackrun.yaml` first, as you would read a Makefile before
running `make`.

## Platform support

| Platform | Status |
|---|---|
| macOS | Supported |
| Linux | Supported |
| Windows | Not yet supported. The binary builds, but `up` reports that Windows is unsupported. |

## Known limitations

- If stackrun itself is killed with SIGKILL, it cannot stop its services,
  and they keep running.
- A process that deliberately moves itself into a new process group or
  session is not reached by stackrun's signals.
- Restart limits (attempts, delays, reset period) are fixed and not yet
  configurable.

## Roadmap

- **Startup dependencies**: start a service only after the services it
  depends on are ready.
- **Health checks**: decide readiness by polling a URL or command.
- **Restart on file changes**: restart a service when its source files
  change.
- **Windows support**.

## Development

Requirements: Go (see [`go.mod`](go.mod)) and
[golangci-lint](https://golangci-lint.run/) v2.

```sh
go build -o bin/stackrun ./cmd/stackrun   # build
go test -race ./...                       # run tests with the race detector
golangci-lint run                         # lint and check formatting
```

The design, concurrency model, and testing approach are described in
[docs/architecture.md](docs/architecture.md). Changes go through pull
requests, and CI runs linting and tests on Linux and macOS.

To regenerate the demo after changing the output, install
[VHS](https://github.com/charmbracelet/vhs) and run `vhs docs/demo.tape`.

## Licence

[MIT](LICENSE)