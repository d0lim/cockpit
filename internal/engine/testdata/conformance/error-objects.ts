function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

async function releasing(body: () => Promise<string>, release: () => Promise<void>): Promise<string> {
	let failed = false;
	try {
		return await body();
	} catch (error) {
		failed = true;
		throw error;
	} finally {
		await release().catch((error: unknown) => {
			if (!failed) throw error;
		});
	}
}

export default async function (): Promise<void> {
	const error = new Error("status 409");
	check("message, name and instanceof", error.message === "status 409" && error.name === "Error" && error instanceof Error);
	check("TypeError is also an Error", new TypeError("t") instanceof Error);
	let thrown: unknown;
	try {
		throw "plain";
	} catch (caught) {
		thrown = caught;
	}
	check("a non-Error value can be thrown", thrown === "plain");
	const wrapped = `${(error as Error).message} · evidence`;
	check("concatenates messages", wrapped === "status 409 · evidence");
	const ok = await releasing(async () => "body", async () => {});
	check("value survives the await in finally", ok === "body");
	let kept = "";
	await releasing(
		async () => {
			throw new Error("body failed");
		},
		async () => {
			throw new Error("release failed");
		},
	).catch((caught: Error) => {
		kept = caught.message;
	});
	check("release failure does not mask the original failure", kept === "body failed");
}
