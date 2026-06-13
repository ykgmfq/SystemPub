# SystemPub

Go application that publishes system metrics (ZFS pool status, snapshots) to MQTT for Home Assistant autodiscovery.

For additional context on installation, configuration, and updates, consult the [project wiki](https://github.com/ykgmfq/SystemPub/wiki).

## Project layout

- `models/` — shared type and model definitions (no business logic)
- `sanoid/` — ZFS/sanoid integration; types in `models.go`, logic in `sanoid.go` and `zpool.go`
- `mqttclient/` — MQTT client wrapper
- `systemd/` — systemd integration
- `main.go` — entry point and wiring

## Code conventions

**Type/model definitions go in their own files**, separate from business logic.
In each package, put structs, constants, and type declarations in a dedicated `models.go` (or equivalent).
Keep `<package>.go` for functions and logic only.

**Always check errors and escalate them.**
Never silently discard an error.
Functions use `result, err` return style.
If an error from a called function cannot be handled at the current level, return `nil, err` to escalate it unchanged.

```go
// correct
result, err := doSomething()
if err != nil {
    return nil, err
}

// wrong
result, _ := doSomething()
```

**Favour functional style; keep functions pure.**
Functions should compute and return results from their inputs without side effects.
Avoid mutating state passed in, relying on package-level variables, or mixing I/O with logic.
Where side effects are unavoidable (I/O, channels), isolate them at the call site rather than burying them inside logic functions.

```go
// correct — pure: input in, result out
func buildEntries(pool *Pool, interval time.Duration) []Entry { ... }

// wrong — impure: writes to channel inside logic
func buildAndPublish(pool *Pool, pubs chan *paho.Publish) { ... }
```

**Never create files via Bash heredocs (`cat << EOF`) or inline shell generation.**
Use the Write or Edit tools to create files, commit them to the repo, and copy them in scripts.

**In justfile recipes, express dependencies as just dependencies, not `just <recipe>` calls.**
Use parameterized dependencies (`(recipe arg)`) when the dependency takes arguments.

**Keep the top-level of the repo clean.**
Organize files into subdirectories unless impractical (e.g. `go.mod`, `justfile`, and `main.go` must live at the root).
Deployment artifacts go in `deploy/`.

**In markdown files, each sentence starts on its own line.**
This keeps git diffs clean — a change to one line only touches one sentence.

**Human-facing text uses proper prose — clear, concise, natural language.**
This applies to markdown files, comments, commit messages, and any other text a person reads.
Write in full sentences, avoid terse shorthand, and prefer plain words over jargon.

**Keep implementation details in comments and markdown to a minimum.**
State intent and rationale — the why — rather than restating how the code works.
Mechanics duplicated in prose drift out of sync with the code they describe, so let the code be the source of truth.

**In GitHub Actions workflows, put longer `run:` blocks into shell script files.**
Keep one-liners inline.
Scripts live in `.github/scripts/` and are referenced from the workflow step.

**In shell scripts, avoid line continuations with backslash.**
Use intermediate variables instead.

## Commands

```sh
just build   # build binary
just test    # run tests
just run     # build and run
```
