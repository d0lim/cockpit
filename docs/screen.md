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
| `/` | Runs | What is happening on this hub now? | The queue and the active run, the kept pool with its five places, the ended runs, and a form that submits a run. A row is the run id, suite, commit, lifecycle and verdict, and links to the run's default screen. |
| `/runs/{id}` | Overview | What state is this run in, and what can I do? | Lifecycle and verdict, the reasons and notices, the actions the run allows, and the pipeline stages it passed. |
| `/runs/{id}/report` | Report | Did it pass, and if not, why? | The verdict and its reasons, the stage → flow → step table and what was not counted as passed. |
| `/runs/{id}/report/{flow}` | Flow | What did this flow do, and where did it stop? | The flow's steps, each with the requests it sent, the ids it marked and what the environment recorded under those ids, and the source line of the picked step. |
| `/runs/{id}/signals/{view}` | Signals | What happened inside the environment while the suite ran? | One of four views (`trace`, `events`, `table`, `logs`) and the collector status. |
| `/runs/{id}/env` | Environment | How do I get into this environment? | The service ports with their open modes, the kit, the exec command and the flows it can run again, and the report bundle path. |
| `/runs/{id}/pipeline` | Pipeline | What did each stage print? | The masked pipeline output lines. |
| `/hub` | Hub | Which hub is this? | Its registry id and name, state directory, address, version, served host APIs, limits and Docker state. |
| `/stacks` | Other stacks | What else uses this host's resources? | The running compose projects without the cockpit label and the unowned orphans, read only, with the command that removes each by hand. |

- A run screen has a short header (run id, lifecycle, verdict) and the run's screen links: Overview, Report, Signals, Environment, Pipeline. The header is the only part every run screen shares.
- A run's default screen is Overview while it is queued, Signals while it is active, and Report once it has ended. A run whose host API this hub does not serve has only Overview and Pipeline.
- An unknown run id answers 404 with a screen that says so; a malformed one answers 400.

## Live connection

`GET /live?screen=<path>` upgrades to a WebSocket. `screen` is the path of the screen the tab shows.

- The guard refuses the upgrade unless `Origin` is `http://127.0.0.1:<port>` or `http://localhost:<port>` (403, reason `origin`), the same origins it accepts for `POST`. A browser always sends `Origin` on a WebSocket handshake.
- Each screen declares its live regions, each an element with a fixed id. On connect, the hub renders every live region of the screen and sends them as `<hx-partial hx-target="#<id>" hx-swap="outerMorph">` elements. After that, a bus event or a new signal marks the regions it touches as changed, and the hub sends the changed regions at most every 250 milliseconds.
- Because every message carries whole regions, a reconnect needs no cursor and repeats nothing: the first message after it brings each region up to date, and the morph keeps scroll positions and open details.
- The tab closes the connection while it is hidden (`ws.pauseOnBackground`) and reconnects when it is shown.
- The hub closes the connection with code 1000 when `screen` names no screen, or when the screen's run is gone from the list, so the tab does not reconnect.
- A connection that falls behind the bus is dropped by the bus; the hub subscribes it again and marks every region changed.
- The code is `live.Handler`. A screen hands it a `live.Screen`: its run, its regions, the regions each bus event kind touches, the regions new signals touch, and the renderer.

| Screens | Region | Changed by |
|---|---|---|
| every screen | `rail` | `runs` |
| every run screen | `run-head` | `runs` |
| Runs | `runs` | `runs` |
| Hub | `hub` | `runs` |
| Other stacks | `stacks` | `runs` |
| Overview | `run-overview` | `runs` |
| Report | `report` | `runs` |
| Flow | `flow` | `runs`, new signals |
| Signals | `signals` | new signals |
| Signals | `collectors` | `runs`, new signals |
| Environment | `env` | `runs` |
| Pipeline | `pipeline` | `job` |

`TestTheScreenDocumentTablesMatchTheViewsBothWays` compares both tables on this page with the views the screen registers.

### Screen state in the URL

What a screen shows besides its run lives in its query, so a reload, a pasted URL and the live connection render the same thing:

- Flow: `step`, the picked step; its line is marked in the code view and scrolled to once.
- Signals: `trace` and `span` for the trace view, `f` (repeatable, `key:value`), `q` (text search) and `event` for the events view, `hide` (repeatable, `<table>.<column>`) for the table view, `source` for the logs view.

Signals are read once per run into a cache that keeps the latest 20,000 and reads only the new ones on each render; a run nobody reads for ten minutes leaves the cache. The events view shows the latest 500 events, a table the latest 500 rows, the logs view the latest 2,000 lines of one source.

## Reading signals

- **Correlation.** The ids of a run are the values that a step marked with `ref` and the values of `ref.` attributes (`docs/signals.md`), except masked ones. A signal carries an id when one of its attribute values, or a top-level field of its JSON object body, equals it. Ids that one signal declares together (the refs of one step, or the `ref.` attributes of one row) belong to one group, and groups that share an id merge, so a row that declares a movement id and its transaction id joins the step that marked the movement to the rows that carry the transaction. A picked span, event or table row shows its `ref.` attributes and the other ids it carries as links to the events view searched for that id, then lists the related signals: the steps that marked an id of the same group, then up to 40 other signals that carry one, each linking to where it is shown (a step to the Flow screen, a span to the trace view, a log line to its log source, any other event to the events view). A table whose rows carry an id of a group that a step marked gets a `step` column linking to those steps. On the Flow screen the picked step lists the requests it sent (each linking to its span), the ids it marked and the signals related to them, so a database row leads back to the request that made it.
- **Table view.** Its columns are the signal attributes other than `table` and the `ref.` keys, then, when a signal's body is a JSON object, the keys of that object, so a collector that puts a database row in the body shows the row's columns.
- **Events view.** `q` keeps the events whose collector, id, attributes or body contain the text, ignoring case, among all the events read; the filters and the window of 500 apply after it. The detail shows a JSON object body indented.
- **After destroy.** A destroyed run's Signals screen reads the frozen copy in its report bundle and says so.

## Actions

The run actions are forms that post to `POST /runs/{id}/do/{op}`, where `op` is one of `keep`, `unkeep`, `stop`, `resume`, `destroy`, `abort` and `exec` (with the form field `flow`). They pass the same guard as `/api/` writes: the page sets `X-Cockpit: 1` on every htmx request (`hx-headers:inherited`) and the browser sends `Origin`.

- The hub runs the operation through the same scheduler call as `POST /api/runs/{id}/{op}`, with the actor `ui`, and answers with a notice fragment: the result on success, or the problem message and code with the problem's status on refusal. htmx 4 swaps 4xx answers, so a refusal shows where the action was.
- The run's new state reaches the page through the live connection, not through the action's answer.
- Destroy asks for confirmation with `hx-confirm`.
- The Runs screen submits a run with `POST /runs/submit`: a repository from the hub's `allowedRepositories`, a commit (default `HEAD`) and a suite chosen from the suites that the repository declares at that commit (`GET /runs/suites?worktree=<path>&commit=<ref>` answers the options again whenever the repository or the commit changes). It runs the same submission as `POST /api/runs`, with the submitter `ui`, and answers with a notice that links to the new run.

## Assets

`web/` builds the browser assets into `internal/hub/ui/dist`, which the hub serves under `/assets/`:

- `htmx.min.js` and `hx-ws.min.js` from the pinned `htmx.org` package, copied unchanged.
- `app.css`, built by Tailwind CSS from `web/screen/app.css` and the classes in the templ components.
- `app.js`, built from `web/screen/app.ts` as a classic script: it shows the live connection's state and copies `data-copy` text to the clipboard.

Every page sends `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`. The screen uses no `hx-on` and no `js:` values, so htmx never evaluates text; `includeIndicatorCSS` is off and the indicator styles live in `app.css`.
