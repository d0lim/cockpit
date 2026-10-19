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
		"Ctx":         "The ports a step can use. Everything a flow touches goes through here; there is no fetch, file system or Node API.",
		"Ctx.env":     "The public env, read from env.json.",
		"Ctx.priv":    "The `local` section of the private env. Every value is masked as **** in the console, failure messages and output files.",
		"Ctx.http":    "Requests to the origins listed in env.json.",
		"Ctx.capture": "Stores a value for later steps and flows in this run. Captured values are masked in all output.",
		"Ctx.get":     "Reads a captured value. Throws, failing the step, when the value was never captured.",
		"Ctx.totp": "Returns a TOTP code for an account in env.json `totpSeedKeys`, whose seed is in the private env. " +
			"It does not return the same code twice for one account, and waits for the next time step when it has to.",
		"Ctx.saveSeed":   "Stores a TOTP seed for an account in env.json `totpSeedKeys` into the private env file.",
		"Ctx.crypto":     "Randomness and RSA signing.",
		"Ctx.readSource": "Reads a regular file under /work/source/<source>, at most 4 MiB. Links and paths outside the source are refused.",
		"Ctx.listSource": "Lists the entries of a directory under /work/source/<source>; an absent directory lists as empty.",
		"Ctx.clock":      "The current time in milliseconds since the epoch.",
		"Ctx.sleep":      "Pauses for `ms` milliseconds.",
	}
}
