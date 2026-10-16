function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default async function (): Promise<void> {
	const all = await Promise.all([Promise.resolve(1), 2, (async () => 3)()]);
	check("Promise.all collects in order", all.join(",") === "1,2,3");
	const settled = await Promise.allSettled([Promise.resolve("ok"), Promise.reject(new Error("no"))]);
	check(
		"Promise.allSettled collects states",
		settled[0].status === "fulfilled" && settled[1].status === "rejected" && (settled[1] as PromiseRejectedResult).reason.message === "no",
	);
	let rejected = "";
	await Promise.all([Promise.reject(new Error("first")), Promise.resolve(1)]).catch((error: Error) => {
		rejected = error.message;
	});
	check("Promise.all rejects with the first rejection", rejected === "first");
	check("Promise.race", (await Promise.race([new Promise((r) => setTimeout(() => r("slow"), 20)), Promise.resolve("fast")])) === "fast");
}
