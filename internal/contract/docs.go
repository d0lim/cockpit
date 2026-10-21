package contract

func Docs() map[string]string {
	return map[string]string{
		"ManifestSource":        "Where a flow or step is defined; the runner fills it in.",
		"StepKind":              "`request` sends requests, `wait` waits until a condition holds, `capture` stores values for later steps.",
		"HttpResponse":          "A response as the runner received it. `body` is the raw text, at most 8 MiB. Header names are lowercase.",
		"SendOptions.json":      "Serialized as the request body; `content-type` defaults to `application/json`.",
		"SendOptions.body":      "A raw request body. Ignored when `json` is set.",
		"SendOptions.timeoutMs": "Timeout for this request. Defaults to 30 seconds.",
		"HttpPort.send": "Sends one request and resolves with the response, whatever its status. The URL must be absolute and its origin " +
			"(scheme://host[:port]) must appear as a value of its own in env.json; an IP address or a URL with a path is not an origin. " +
			"Other origins and redirect responses are refused.",
		"CaptureOptions.persist": "Also keep the value in context.json, so later runs and `--flow` runs in the same environment can read it.",
		"CryptoPort":             "Randomness and RSA signing. Keys are opaque handles and never leave the runner.",
		"StageDef":               "A group of flows in the results. `notProven` marks a stage this environment cannot prove, with the reason.",
		"StepDef.kind":           "What the step does; used only for reading the results.",
		"StepDef.run":            "Runs the step. The step fails when this throws; the message is masked and cut to 500 characters.",
		"FlowDef.id":             "Lowercase letters, digits and hyphens, starting with a letter; 2 to 41 characters.",
		"FlowDef.stage":          "The id of a stage declared in the suite.",
		"FlowDef.once": "Runs at most once per environment, for setup such as creating the first admin. With `--flow`, a once target " +
			"that already ran is refused, and a once require that passed is reused instead of rerun.",
		"FlowDef.requires":  "Flows that must pass first. When one fails or is skipped, this flow is skipped.",
		"SuiteDef.name":     "Must equal the file name: the suite in suites/smoke.ts is named `smoke`.",
		"suite":             "Declares the suite. Default-export the result from suites/<name>.ts.",
		"flow":              "Declares a flow: steps run in order, and the first step that throws stops the flow.",
		"step":              "Declares a step.",
		"stage":             "Declares a stage.",
		"PollOptions.what":  "Names the condition in the failure message.",
		"PollOptions.now":   "Pass `ctx.clock`.",
		"PollOptions.sleep": "Pass `ctx.sleep`.",
		"target":            "Formats `METHOD /path` without the query, for failure messages.",
		"expectStatus":      "Throws unless the status is one of `expected`. The message names the method and path, never the query or the body.",
		"json":              "Parses the body as JSON. Throws without echoing the body when it is not JSON.",
		"poll": "Calls `attempt` until `done` holds and resolves with the last value and the attempt count. The first attempt runs at once; " +
			"it throws when the next attempt would start after the deadline.",
		"ServiceState.exitCode": "The exit code of a container that exited; absent while it runs.",
		"Ctx":                   "The ports a step can use. Everything a flow touches goes through here; there is no fetch, file system or Node API.",
		"Ctx.env":               "The public env, read from env.json.",
		"Ctx.priv":              "The `local` section of the private env. Every value is masked as **** in the console, failure messages and output files.",
		"Ctx.http":              "Requests to the origins listed in env.json.",
		"Ctx.capture":           "Stores a value for later steps and flows in this run. Captured values are masked in all output, unless `ref` marked them public.",
		"Ctx.ref":               "Marks a value as a public correlation key of the running step under `name`. The value is no longer masked, and the hub shows it as the step span attribute `ref.<name>`. Refuses a value from the private env or one the credential patterns mask.",
		"Ctx.get":               "Reads a captured value. Throws, failing the step, when the value was never captured.",
		"Ctx.totp": "Returns a TOTP code for an account in env.json `totpSeedKeys`, whose seed is in the private env. " +
			"It does not return the same code twice for one account, and waits for the next time step when it has to.",
		"Ctx.saveSeed":   "Stores a TOTP seed for an account in env.json `totpSeedKeys` into the private env file.",
		"Ctx.crypto":     "Randomness and RSA signing.",
		"Ctx.readSource": "Reads a regular file under /work/source/<source>, at most 4 MiB. Links and paths outside the source are refused.",
		"Ctx.listSource": "Lists the entries of a directory under /work/source/<source>; an absent directory lists as empty.",
		"Ctx.clock":      "The current time in milliseconds since the epoch.",
		"Ctx.sleep":      "Pauses for `ms` milliseconds.",
		"EdgeHttpPort.send": "Sends one GET or HEAD request to an edge port of this run and resolves with the response. The URL is " +
			"http://<edge name>/path, where the edge name is an offset name from the project declaration; ports, IP addresses, " +
			"other hosts and redirect responses are refused.",
		"SqlPort.query": "Runs one read statement (select, with, show, explain, describe, table, values) against a database of the " +
			"declaration, in a read-only transaction that is rolled back. A second statement, a write, or a MySQL `INTO OUTFILE` is " +
			"PORT_DENIED; the refusal never quotes the statement.",
		"SqlOptions.params": "Values bound to the placeholders of the statement in order: `?` for MySQL, `$1` to `$n` for PostgreSQL. " +
			"They travel to the driver as parameters, never as SQL text, so ids from earlier steps need no quoting. Only strings, " +
			"numbers, booleans and null are accepted; `undefined`, holes, objects and arrays are PORT_BAD_ARGS, and so is a count " +
			"that does not match the placeholders. At most 100 values and 64 KiB, counting the bytes of each string and 8 for any other value (PORT_LIMIT). Pass integers above " +
			"2^53 as strings.",
		"SignalPort": "Emits signals into this run's stream under the collector's id. Every string is masked before it is stored. " +
			"The `runner` time source is reserved for the hub.",
		"StatePort.get": "Reads a value this collector stored earlier in this run, or `undefined`.",
		"StatePort.set": "Stores a string of at most 64 KiB under a key of at most 256 characters; the state survives hub restarts. " +
			"The key and the value pass the run redactor first, so a credential is stored and read back masked. The redactor's key-name " +
			"rules also apply inside the value: in `{\"nextPageToken\":\"...\"}` or `token=...` the part after a secret-looking name " +
			"(`*token`, `password`, `secret`, `credential`, `session`, `api_key`) reads back as `****`. The state key itself does not " +
			"trigger them, so `set(\"nextPageToken\", cursor)` keeps the cursor; inside a stored object, use neutral names such as `cursor`.",
	}
}
