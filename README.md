# Datatree — Infinite Driver

Browse data through arbitrary nested containers with a unified interface.

## What it is

Data often lives inside nested containers: a computer has drives, a drive has files, a ZIP archive has entries, a text file has URLs, a URL leads to an HTML page with images. Existing tools handle one or two levels, but not an infinite hierarchy.

Datatree solves this by chaining **drivers** — small programs that each know how to open one kind of container and list what's inside. A ZIP driver opens ZIPs. A directory driver lists folders. A text driver extracts URLs. Stack them together and you can drill down as deep as you like, all through the same interface.

## Build and run

```
./run.sh
```

This builds all drivers and the host, then starts the HTTP server on port 8080. Open `http://localhost:8080` in your browser to explore your home directory.

You can also build and run separately:

```
./build.sh    # compile everything
./host        # start the server
```

## Architecture

### The stack model

Each driver sits on top of another, requesting bytes from the layer below and presenting structured resources to the layer above.

```
root_driver              -> lists home directory entries
  user clicks a folder
directory_driver         -> asks root_driver for the folder path
                         -> lists that folder's entries
  user clicks a .zip file
zip_driver               -> asks directory_driver for the ZIP bytes
                         -> parses the archive, lists entries
  user clicks a .txt entry
text_driver              -> asks zip_driver for the text content
                         -> extracts URLs, lists them
```

New layers are added on demand: when the user clicks a resource to expand it, the host spawns the appropriate driver, which requests data from its parent and emits its own list of children. The stack grows one layer per expansion, with no limit on depth.

### The driver interface

Every driver is a standalone executable that communicates with the host via JSON lines on stdin/stdout. Two phases:

**Phase 1 — initialization (required).** Driver may optionally request bytes from its parent:

```
-> stdout: { "cmd": "range", "offset": 0, "length": -1 }
<- stdin:  { "data": "<base64>", "size": 2048 }
```

Then it must emit exactly one `resources` message:

```
-> stdout: { "resources": [ { "name": "...", "kind": "...", "size": 123 }, ... ] }
```

Even an empty driver sends `{ "resources": null }`.

**Phase 2 — serve child ranges (optional).** After emitting resources, the driver may stay alive and handle incoming range requests for its children:

```
<- stdin:  { "cmd": "range", "offset": 0, "length": -1, "child": "some/entry" }
-> stdout: { "data": "<base64>", "size": 4096 }
```

Leaf drivers (e.g. `text_driver`) exit after Phase 1. Non-leaf drivers stay in a loop.

### Available drivers

| Driver | Input | Output resources | Serves child ranges? |
|---|---|---|---|
| `root_driver` | None (reads home directory) | Files and directories with kind | Yes |
| `directory_driver` | Directory path from parent | Files and subdirectories | Yes |
| `zip_driver` | ZIP file bytes from parent | ZIP archive entries | Yes |
| `text_driver` | Text content from parent | Extracted URLs | No (leaf) |

### Resource kinds and routing

Resource kinds drive the auto-load mapping in the host:

- `directory` -> `directory_driver`
- `text` -> `text_driver`
- `archive` / `zip` -> `zip_driver`
- `image`, `pdf`, `html`, `json`, `csv`, `file` -> `text_driver` (default)

### Host HTTP API

| Endpoint | Method | Description |
|---|---|---|
| `/` | GET | Web UI |
| `/api/drivers` | GET | List available driver executables |
| `/api/list` | GET | List root-level resources |
| `/api/autoLoad` | POST | Load appropriate driver for a handle, returns child resources |
| `/api/data` | GET | Fetch raw bytes from a handle (`?handle=&offset=&length=`) |

## Writing a new driver

1. Write an executable that speaks JSON lines on stdin/stdout.
2. On startup, optionally request bytes from upstream with `{"cmd":"range", ...}`.
3. Process the data and emit `{"resources": [...]}`.
4. Optionally stay alive to serve `{"cmd":"range", "child":"..."}` requests.
5. Place the compiled binary in the `drivers/` directory.
6. Add a kind mapping in the host's `autoLoad` handler so it knows when to use your driver.

## Project structure

```
host.go                    HTTP server and driver orchestrator
web/index.html             HTMX-based browser UI
drivers_go/                Driver source code (Go)
  root_driver.go
  directory_driver.go
  zip_driver.go
  text_driver.go
drivers/                   Compiled driver binaries (built by run.sh)
spec.md                    Project specification
AGENTS.md                  Coding-style and SPEC notes for agents
```
