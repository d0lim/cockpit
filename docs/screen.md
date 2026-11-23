# Screen

This page holds the rules of the hub's screen: the sidebar, the run header, the screens and the question each one answers, the live connection and the actions. The code is in `internal/hub/screen` (handlers, view models and templ components) and `web/` (the stylesheet and the browser assets). When this page and the code disagree, this page is fixed first and the code follows.

## Principles

- **One question per screen.** Each screen answers one question about the hub or a run and holds only what that answer needs. The question is the reason the screen exists; the screen does not print it.
- **Pick on the left, read on the right.** Where a screen offers things to pick (the flows and steps of a run, the traces of a run), the list stands on the left and scrolls on its own, and what is picked shows on the right. The picked row is scrolled into view inside its own list, never by moving the page.
- **State and actions stay in view.** The sidebar and the run header do not scroll away, so the hub's state, the run's verdict and the actions the run allows are on every screen.
- **The server renders.** Every screen is HTML rendered by templ components in the hub process. The browser holds no model of runs or signals; it holds the page the hub sent and the swaps the hub pushes.
- **htmx 4 drives navigation and updates.** Links and forms are boosted (`hx-boost:inherited`), so moving between screens swaps the page body and pushes the URL. Every screen also answers a plain `GET` with a whole page, so a reload or a pasted URL shows the same screen.
- **One live connection per tab.** A tab opens one WebSocket (`hx-ws:connect`) to the hub. The hub pushes `<hx-partial>` elements that replace the live regions of the screen the tab shows.
- **The screen reads only what the API reads.** Every value on a screen comes from the same view builders that answer `/api/` (`docs/api.md`), so it passed the same run redactor. The screen adds no source of run data of its own.

## Sidebar

The left column is the sidebar. It names this hub and leads to everything the hub holds, from every screen.

- **This hub** is first: its name and address. A line under it says when Docker does not answer or the last tick failed.
- **Runs**, **Other stacks** and **Hub** are the hub's screens. Other stacks carries the count of compose projects on the host that no registered hub owns.
- **Now** lists the queue and the active run, when there is one, each linking to the run's default screen.
- **Kept environments** lists the kept pool with its count and its size, each run with whether it is up or stopped, and the count of running environments against the running cap.
- **Hubs** lists the other entries of the hub registry. A hub whose lock names a live process is shown as up and links to its own screen (`http://127.0.0.1:<port>/`); a hub that is not running is shown as down and links to the Hub screen, which holds the command that starts it. This hub never acts on another hub's runs: their records and report bundles live in the other hub's state directory, and the other hub's origin differs, so the guard refuses its requests.
- The state of the live connection is last.

In a window narrower than 1024 pixels the sidebar is a strip across the top that keeps this hub and its three screens.

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
| `/` | Runs | What is happening on this hub now? | The kept pool and the running cap as meters, the queue and the active run, the kept runs, the ended runs, a form that submits a run, and a form that imports a bundle archive. A row is the verdict, suite and commit, run id, submitter, when it finished and how long it took, lifecycle, and for a run that did not pass the reason it kept. It links to the run's default screen. |
| `/runs/{id}` | Overview | What state is this run in? | Where the run stands and why, the notices, where a failed run stopped, the pipeline stages it passed and the facts of its submission. |
| `/runs/{id}/report` | Report | Did it pass, and if not, why? | The flow tree, and beside it the verdict and its reasons, the step counts, where the run stopped and what was not counted as passed. |
| `/runs/{id}/report/{flow}` | Flow | What did this flow do, and where did it stop? | The flow tree, and beside it the picked flow with its steps, or the picked step with its message, the requests it sent, the ids it marked and what the environment recorded under those ids, or its line in the suite source. |
| `/runs/{id}/signals/{view}` | Signals | What happened inside the environment while the suite ran? | One of four views (`trace`, `events`, `table`, `logs`) and the collector status. |
| `/runs/{id}/wire` | Wire | What was each server asked, and what did it answer? | For a run submitted with `wire` ([wire record](wire.md)): the flow tree with call counts, and beside it the state of the record, the calls of the picked step or flow as a sequence diagram, a trace or a table, and the picked call's request and response. |
| `/runs/{id}/env` | Environment | How do I get into this environment? | The service ports with their open modes, the kit, the exec command and the flows it can run again, and the report bundle path. |
| `/runs/{id}/pipeline` | Pipeline | What did each stage print? | The masked pipeline output lines. |
| `/hub` | Hub | Which hub is this? | Its registry id and name, state directory, address, version, served host APIs, limits and Docker state, and the hubs on this host. |
| `/stacks` | Other stacks | What else uses this host's resources? | The running compose projects without the cockpit label and the unowned orphans, read only, with the command that removes each by hand. |

### Run header

Every run screen starts with the run header, which stays at the top of the window while the screen scrolls. It is the only part every run screen shares.

- The first line is a link back to Runs, the run id, the verdict, the lifecycle, `keep` when it is on, and the actions the run allows now ([Actions](#actions)).
- The second line is the suite, the commit, the submitter, when it was submitted, how long it ran, and `wire` for a run submitted with it.
- The third line is the run's tabs: Overview, Flows, Signals, Environment, Pipeline. Flows holds three screens, Report, Flow and Wire, which share the flow tree. A run whose host API this hub does not serve has only Overview and Pipeline.
- A run's default screen is Overview while it is queued, Signals while it is active, and Report once it has ended.
- While a run is active, Overview says `Running the <stage> stage since <time>`. The time is the start of the current stage from the run's stage times (`stageStartedAt` in `RunItem`, `docs/signals.md`), or the run's start when the record holds no open entry for the current stage.
- An unknown run id answers 404 with a screen that says so; a malformed one answers 400.

### Flows

Report, Flow and Wire are one explorer: the flow tree on the left, what is picked on the right.

- **The flow tree** lists the suite's stages, each stage's flows with their outcome and `passed/steps`, and under the picked flow its steps with their outcome. On Report and Flow a step carries how long it took. On Wire the tree lists the flows and steps that the picked take ran, with the outcomes they had in that take and each step's count of calls, and ends with `Outside any step`; a take of an exec lists only what the exec ran, and a step that did not run is absent. The tree scrolls on its own and shows the picked row.
- `only=failed` and `only=skipped` leave in the tree the flows that hold such a step, with those steps listed; the counts of all, failed and skipped steps are the links that set it.
- **Nothing picked** (Report) shows the run's result: the verdict and the seal check (with a `network open` mark when the commit declares the [open network level](sealing.md#network-level)), the step counts, where the run stopped with the message of that step, the reasons, the collection that failed, what was not counted as passed grouped by flow, and the report bundle path with a link that downloads the run's [bundle archive](export.md). The link is a plain download of `GET /api/runs/{id}/export`, outside the boosted navigation.
- **A picked flow** shows its title, stage, what it requires and whether it can run again, then one of its panes: `Steps` (each step with its outcome, kind, facts and message), `Calls` or `Source`.
- **A picked step** shows its title, outcome, kind and facts, and its message when it has one, then one of its panes: `Step` (the requests it sent, each linking to its span, the ids it marked and the signals related to them), `Calls` or `Source` (the suite source with the step's line marked).
- `Calls` is the Wire screen for the same flow or step, so it is offered only for a run submitted with `wire` and for a flow or step that ran, and the Wire path answers 404 for any other run. The other panes are the Flow screen; `pane=source` picks Source.
- On Wire, a take other than `suite` shows the flow or step with the outcome it had in that take and without the panes, because Step and Source describe the suite's run.
- A run submitted with `wire` says so in the run header, and Report says the state of its record.

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
| every screen | `sidebar` | `runs` |
| every run screen | `run-head` | `runs` |
| Runs | `runs` | `runs` |
| Hub | `hub` | `runs` |
| Other stacks | `stacks` | `runs` |
| Overview | `run-overview` | `runs` |
| Report | `report` | `runs` |
| Flow | `flow` | `runs`, new signals |
| Signals | `signals` | new signals |
| Signals | `collectors` | `runs`, new signals |
| Wire | `wire` | `runs` |
| Environment | `env` | `runs` |
| Pipeline | `pipeline` | `job` |

`TestTheScreenDocumentTablesMatchTheViewsBothWays` compares both tables on this page with the views the screen registers.

### Screen state in the URL

What a screen shows besides its run lives in its query, so a reload, a pasted URL and the live connection render the same thing:

- Runs: `verdict` (`passed`, `failed`, `error` or `aborted`) keeps the runs with that verdict, and `all` lists every ended run instead of the latest 30.
- Report and Flow: `only` (the flow tree's filter), `step` (the picked step) and `pane` (`source`). The step's line is marked in the source and scrolled to once, inside the source.
- Wire: `only`, `take`, `flow` and `step`, `scope=flow` or `scope=outside`, `target`, `offset`, `view`, `ties`, `open` and `call`. Without a selection the screen opens on the step the API picks (`docs/api.md`). The region `wire` holds the notices, the flow tree and the calls and reads only the cached index; the picked call sits outside it, so a `runs` event never reads a body.
- The picked call opens in a panel along the right edge of the window. In a window at least 1280 pixels wide the screen makes room for the panel, so the calls stay visible beside it; in a narrower one the panel lies over them. The panel starts with the method and path, the status, how long the call took, the sizes and who called whom; then the request and the response, each with its body first and its headers folded; then what ties the call to its step. Picking a call keeps the page where it was: the picked row or diagram line is scrolled into view once, and the panel's close link, the Escape key or a click outside the panel on nothing that can be picked drops `call` and leaves the page in place.
- The Wire screen draws one selection three ways, chosen with `view`, and they answer the same question. `sequence` (the default) shows who called whom in order: one lifeline per service in order of first appearance with the runner first, a request and a dashed answer per call, and a note where 150 ms or more passed. Its vertical axis is order, not time. `trace` shows what caused what and where the time went: trees of calls grouped by what started them, on a time axis that folds idle stretches. `calls` is the table. In a flow selection the sequence is cut into one band per step.
- Color and arrowhead say what ties a call to its step: blue for `exact` and `traced`, grey for `nested`, an open purple head for `background` and an open grey head for `time`. A failed call is red whatever its link. `ties=only` leaves out `background`, `time` and `none` calls.
- The sequence and the trace fold a call that repeats at least three times under the same parent and in the same step into its first occurrence with `×N`, comparing caller, target, method, status, link and the path with its numbers and identifiers removed, and the calls below it. `open=<seq>` unfolds one group. The diagrams are SVG that the server lays out: positions are SVG attributes, so the page needs no inline style, and each call is a link. htmx boosts only HTML links, so the screen script follows a link inside a diagram through a boosted link of the page, and picking a call in a diagram swaps the page like any other link.
- `Copy as Mermaid` copies the diagram shown as Mermaid `sequenceDiagram` text with the existing copy control. The hub does not render Mermaid: the library draws in the browser and injects styles the content security policy refuses. The route filters name the routes, with each route's count for the whole take in their title; the `suite` take is listed first.
- Signals: `trace`, `span` and `open` (repeatable, a span id, or `all`) for the trace view, `f` (repeatable, `key:value`), `q` (text search) and `event` for the events view, `hide` (repeatable, `<table>.<column>`) for the table view, `source` for the logs view.

Signals are read once per run into a cache that keeps the latest 20,000 and reads only the new ones on each render; a run nobody reads for ten minutes leaves the cache. The events view shows the latest 500 events, a table the latest 500 rows, the logs view the latest 2,000 lines of one source.

## Reading signals

- **Correlation.** The ids of a run are the values that a step marked with `ref` and the values of `ref.` attributes (`docs/signals.md`), except masked ones. A signal carries an id when one of its attribute values, or a top-level field of its JSON object body, equals it. Ids that one signal declares together (the refs of one step, or the `ref.` attributes of one row) belong to one group, and groups that share an id merge, so a row that declares an order id and its payment id joins the step that marked the order to the rows that carry the payment. A picked span, event or table row shows its `ref.` attributes and the other ids it carries as links to the events view searched for that id, then lists the related signals: the steps that marked an id of the same group, then up to 40 other signals that carry one, each linking to where it is shown (a step to the Flow screen, a span to the trace view, a log line to its log source, any other event to the events view). A table whose rows carry an id of a group that a step marked gets a `step` column linking to those steps. On the Flow screen the picked step lists the requests it sent (each linking to its span), the ids it marked and the signals related to them, so a database row leads back to the request that made it.
- **Trace view.** The root of a run trace is the hub's `pipeline` span, its children are the stage spans, and the runner's `run` tree sits under `stage:suite` (`docs/signals.md`). The trace list names each trace by its first root, so a run trace is listed as `pipeline`. A span whose parent is not in the trace is shown as a root, so a run reported before the hub wrote stage spans still shows its `run` tree. A span whose time source differs from the root's is marked with `R` (runner), `C` (container) or `H` (host), so the runner spans under the host-timed stages carry `R`. A service span of the collector `otlp` sits under the call span whose request started it once the report has reparented it ([OTLP signals](signals.md#otlp-signals)), and carries `C` because its times come from the service's clock. A service span whose trace id the hub did not map (a healthcheck request, a job the service started on its own) forms its own trace, named by its first root and marked `service`. The trace list puts exec traces first, then the run trace, then these service traces, each group newest first, and lists the 20 newest service traces (and the picked one), with one row that counts the rest, so a kept run whose healthchecks start a trace every few seconds still shows its run trace near the top. The traces are listed on the left and the picked trace's spans on the right. A trace opens to its fourth level: a deeper span shows once its parent is opened with `open=<span>`, and a row with hidden children carries their count and the link that opens or closes it. The ancestors of the picked span are open, and `open=all` opens every row. The picked span's detail stays in view while the spans scroll: beside them where there is room for both, above them where there is not.
- **Table view.** Its columns are the signal attributes other than `table` and the `ref.` keys, then, when a signal's body is a JSON object, the keys of that object, so a collector that puts a database row in the body shows the row's columns. A metric adds a `value` column, and a `unit` column when it carries one, and a histogram adds `count` and `sum`; in these columns the metric's own fields win over attributes of the same name. The rows are the winning revisions, so a metric series with an `id` is one row that shows its latest total.
- **Events view.** `q` keeps the events whose collector, id, attributes or body contain the text, ignoring case, among all the events read; the filters and the window of 500 apply after it. The detail shows a JSON object body indented. The picked event's detail stays in view while the events scroll, beside them or above them like a span's.
- **After destroy.** A destroyed run's Signals screen reads the frozen copy in its report bundle and says so.

## Actions

The run actions are forms that post to `POST /runs/{id}/do/{op}`, where `op` is one of `keep`, `unkeep`, `stop`, `resume`, `destroy`, `abort` and `exec` (with the form field `flow`). They pass the same guard as `/api/` writes: the page sets `X-Cockpit: 1` on every htmx request (`hx-headers:inherited`) and the browser sends `Origin`.

- The run header holds the actions on every run screen, and only the ones the run allows now: `Keep` or `Release keep` unless the run has no environment left, and `Stop`, `Resume`, `Abort` and `Destroy` when the run's allowed events hold them. A run that allows none says so instead. The Environment screen repeats `Resume` beside the ports of a stopped environment.
- The hub runs the operation through the same scheduler call as `POST /api/runs/{id}/{op}`, with the actor `ui`, and answers with a notice fragment: the result on success, or the problem message and code with the problem's status on refusal. htmx 4 swaps 4xx answers, so a refusal shows too. The notice appears at the bottom right of the window, outside every live region, and stays until the next action or screen.
- The run's new state reaches the page through the live connection, not through the action's answer.
- Destroy asks for confirmation with `hx-confirm`.
- The Runs screen submits a run with `POST /runs/submit`, from the form that `New run` opens: a repository from the hub's `allowedRepositories`, a commit (default `HEAD`) and a suite chosen from the suites that the repository declares at that commit (`GET /runs/suites?worktree=<path>&commit=<ref>` answers the options again whenever the repository or the commit changes), and a box that records the run's service calls ([wire record](wire.md)). It runs the same submission as `POST /api/runs`, with the submitter `ui`, and answers with a notice that links to the new run.

- The Runs screen imports a run with `POST /runs/import`, from the form that `Import` opens: one file, the bundle archive or the zip of a GitHub Actions artifact that holds one, sent as `multipart/form-data` in the field `archive`. It runs the same import as `POST /api/runs/import` ([importing](export.md#importing)). On success it answers `HX-Redirect` with the run's Overview, so the browser opens the imported run; a request without `HX-Request` gets a 303 to the same place. On refusal it answers a notice with the problem message and code, as the other actions do. The form part is limited to the import body limit.

## Assets

`web/` builds the browser assets into `internal/hub/ui/dist`, which the hub serves under `/assets/`:

- `htmx.min.js` and `hx-ws.min.js` from the pinned `htmx.org` package, copied unchanged.
- `app.css`, built by Tailwind CSS from `web/screen/app.css`, the stylesheets it imports and the classes in the templ components.
- `app.js`, built from `web/screen/app.ts` as a classic script. It shows the live connection's state, copies `data-copy` text to the clipboard, scrolls each `data-scroll-here` element into view once, inside its `data-scroll-box` and then in the window when it still lies outside it, keeps the position of each named `data-scroll-box` across a swap, sets `--head-height` to the height of the run header so the lists under it start below it, closes the picked call's panel on the Escape key or a click outside it, and follows the links inside a diagram through a boosted link.

Every page sends `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`. The screen uses no `hx-on` and no `js:` values, so htmx never evaluates text; `includeIndicatorCSS` is off and the indicator styles live in `app.css`.
