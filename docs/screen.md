# Screen

This page holds the rules of the hub's screen: the workspace rail, the screens and the question each one answers, the live connection and the actions. The code is in `internal/hub/screen` (handlers, view models and templ components) and `web/` (the stylesheet and the browser assets). When this page and the code disagree, this page is fixed first and the code follows.

## Principles

- **One question per screen.** Each screen answers one question about the hub or a run and holds only what that answer needs. Anything else is one link away, never on the same screen.
- **The server renders.** Every screen is HTML rendered by templ components in the hub process. The browser holds no model of runs or signals; it holds the page the hub sent and the swaps the hub pushes.
- **htmx 4 drives navigation and updates.** Links and forms are boosted (`hx-boost:inherited`), so moving between screens swaps the page body and pushes the URL. Every screen also answers a plain `GET` with a whole page, so a reload or a pasted URL shows the same screen.
- **One live connection per tab.** A tab opens one WebSocket (`hx-ws:connect`) to the hub. The hub pushes `<hx-partial>` elements that replace the live regions of the screen the tab shows.
- **The screen reads only what the API reads.** Every value on a screen comes from the same view builders that answer `/api/` (`docs/api.md`), so it passed the same run redactor. The screen adds no source of run data of its own.

## Workspace rail

The leftmost column is the workspace rail. A workspace is one hub.

- **This hub** is first. Its mark shows a dot when Docker does not answer or the last tick failed.
- **Other hubs** follow, one per entry of the hub registry other than this hub. A hub whose lock names a live process is shown as up and links to its own screen (`http://127.0.0.1:<port>/`); a hub that is not running is shown as down, with the command that starts it. This hub never acts on another hub's runs: their records and report bundles live in the other hub's state directory, and the other hub's origin differs, so the guard refuses its requests.
- **Other stacks** is last. It lists the compose projects on the host that no registered hub owns.

### Hub registry

A hub registers its state directory when it starts, so hubs started with different `COCKPIT_HOME` values can find each other.

- The registry is the directory `cockpit-hubs` beside the default state directory: `$XDG_STATE_HOME/cockpit-hubs`, or `~/.local/state/cockpit-hubs`. `COCKPIT_REGISTRY` replaces it. It does not move with `COCKPIT_HOME`.
- Each entry is `<id>.json`, where the id is the first 12 hex characters of the SHA-256 of the absolute state directory. The entry holds `{home, name}`; the name is the base name of the state directory. The entry stays after the hub stops, so a stopped hub is still listed.
- A hub is up when `hub.lock` in its state directory names a live process; the lock's port gives its address. An entry whose state directory no longer exists is skipped.
- A labelled compose project without a record in this hub (an orphan, see `docs/lifecycle.md`) belongs to the registered hub whose state directory holds a record of that run id. An orphan that no registered hub holds stays unowned.

The registry changes only what the screen shows. The running cap counts every running compose project that is not one of this hub's runs, owned or not, as before.

## Screens

| Path | Screen | Question | Holds |
|---|---|---|---|
| `/` | Runs | What is happening on this hub now? | The queue and the active run, the kept pool with its five places, the ended runs. A row is the run id, suite, commit, lifecycle and verdict, and links to the run's default screen. |
| `/runs/{id}` | Overview | What state is this run in, and what can I do? | Lifecycle and verdict, the reasons and notices, the actions the run allows, and the pipeline stages it passed. |
| `/runs/{id}/report` | Report | Did it pass, and if not, why? | The verdict and its reasons, the stage → flow → step table and what was not counted as passed. |
| `/runs/{id}/report/{flow}` | Flow | Where and how did this flow fail? | The flow's steps with their requests, responses and expectations, and the source of the failing step. |
| `/runs/{id}/signals/{view}` | Signals | What happened inside the environment while the suite ran? | One of four views (`trace`, `events`, `table`, `logs`) and the collector status. |
| `/runs/{id}/env` | Environment | How do I get into this environment? | The service ports with their open modes, the kit, the exec command and the flows it can run again, and the report bundle path. |
| `/runs/{id}/pipeline` | Pipeline | What did each stage print? | The masked pipeline output lines. |
| `/hub` | Hub | Which hub is this? | Its registry id and name, state directory, address, version, served host APIs, limits and Docker state. |
| `/stacks` | Other stacks | What else uses this host's resources? | The running compose projects without the cockpit label and the unowned orphans, read only, with the command that removes each by hand. |

- A run screen has a short header (run id, lifecycle, verdict) and the run's screen links: Overview, Report, Signals, Environment, Pipeline. The header is the only part every run screen shares.
- A run's default screen is Overview while it is queued, Signals while it is active, and Report once it has ended. A run whose host API this hub does not serve has only Overview and Pipeline.
- Signals views show the latest 500 signals. Older signals are read page by page on request, from `GET /api/runs/{id}/signals`, and are not live.
- An unknown run id answers 404 with a screen that says so; a malformed one answers 400.

## Live connection

`GET /live?screen=<path>` upgrades to a WebSocket. `screen` is the path of the screen the tab shows.

- The guard refuses the upgrade unless `Origin` is `http://127.0.0.1:<port>` or `http://localhost:<port>` (403, reason `origin`), the same origins it accepts for `POST`. A browser always sends `Origin` on a WebSocket handshake.
- Each screen declares its live regions, each an element with a fixed id. On connect, the hub renders every live region of the screen and sends them as `<hx-partial hx-target="#<id>" hx-swap="outerMorph">` elements. After that, a bus event or a new signal marks the regions it touches as changed, and the hub sends the changed regions at most every 250 milliseconds.
- Because every message carries whole regions, a reconnect needs no cursor and repeats nothing: the first message after it brings each region up to date, and the morph keeps scroll positions and open details.
- The tab closes the connection while it is hidden (`ws.pauseOnBackground`) and reconnects when it is shown. A screen that is not a run screen or `/` has no live regions, and its connection stays idle.
- The hub closes the connection with code 1000 when the screen's run is destroyed and gone from the list, so the tab does not reconnect to it.

| Screen | Live regions | Changed by |
|---|---|---|
| every screen | `rail` | `runs` |
| Runs | `runs` | `runs` |
| every run screen | `run-head` | `runs` |
| Overview | `run-overview` | `runs`, `job` |
| Pipeline | `pipeline` | `job` |
| Signals | `signals`, `collectors` | a new signal, `resync`, `signal-refused` |
| Environment | `env` | `runs`, `services` |

## Actions

The run actions are forms that post to `POST /runs/{id}/do/{op}`, where `op` is one of `keep`, `unkeep`, `stop`, `resume`, `destroy`, `abort` and `exec` (with the form field `flow`). They pass the same guard as `/api/` writes: the page sets `X-Cockpit: 1` on every htmx request (`hx-headers:inherited`) and the browser sends `Origin`.

- The hub runs the operation through the same scheduler call as `POST /api/runs/{id}/{op}`, with the actor `ui`, and answers with a notice fragment: the result on success, or the problem message and code with the problem's status on refusal. htmx 4 swaps 4xx answers, so a refusal shows where the action was.
- The run's new state reaches the page through the live connection, not through the action's answer.
- Destroy asks for confirmation with `hx-confirm`.

## Assets

`web/` builds the browser assets into `internal/hub/ui/dist`, which the hub serves under `/assets/`:

- `htmx.min.js` and `hx-ws.min.js` from the pinned `htmx.org` package, copied unchanged.
- `app.css`, built by Tailwind CSS from `web/app.css` and the templ components.

Every page sends `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`. The screen uses no `hx-on` and no `js:` values, so htmx never evaluates text; `includeIndicatorCSS` is off and the indicator styles live in `app.css`.
