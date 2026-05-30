# SystemPub

Go application that publishes system metrics (ZFS pool status, snapshots) to MQTT for Home Assistant autodiscovery.

## Project layout

- `models/` — shared type and model definitions (no business logic)
- `sanoid/` — ZFS/sanoid integration; types in `models.go`, logic in `sanoid.go` and `zpool.go`
- `mqttclient/` — MQTT client wrapper
- `systemd/` — systemd integration
- `main.go` — entry point and wiring

## Code conventions

**Type/model definitions go in their own files**, separate from business logic.
In each package, put structs, constants, and type declarations in a dedicated `models.go` (or equivalent). Keep `<package>.go` for functions and logic only.

**Always check errors and escalate them.**
Never silently discard an error. Functions use `result, err` return style. If an error from a called function cannot be handled at the current level, return `nil, err` to escalate it unchanged.

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
Functions should compute and return results from their inputs without side effects. Avoid mutating state passed in, relying on package-level variables, or mixing I/O with logic. Where side effects are unavoidable (I/O, channels), isolate them at the call site rather than burying them inside logic functions.

```go
// correct — pure: input in, result out
func buildEntries(pool *Pool, interval time.Duration) []Entry { ... }

// wrong — impure: writes to channel inside logic
func buildAndPublish(pool *Pool, pubs chan *paho.Publish) { ... }
```

## Commands

```sh
just build   # build binary
just test    # run tests
just run     # build and run
```
