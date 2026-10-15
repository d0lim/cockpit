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
	const error = new Error("상태 409");
	check("message·name·instanceof", error.message === "상태 409" && error.name === "Error" && error instanceof Error);
	check("TypeError 도 Error 다", new TypeError("t") instanceof Error);
	let thrown: unknown;
	try {
		throw "plain";
	} catch (caught) {
		thrown = caught;
	}
	check("Error 가 아닌 값도 던질 수 있다", thrown === "plain");
	const wrapped = `${(error as Error).message} · 증거`;
	check("메시지를 이어 붙인다", wrapped === "상태 409 · 증거");
	const ok = await releasing(async () => "body", async () => {});
	check("finally 의 await 뒤에 값이 남는다", ok === "body");
	let kept = "";
	await releasing(
		async () => {
			throw new Error("본문 실패");
		},
		async () => {
			throw new Error("해제 실패");
		},
	).catch((caught: Error) => {
		kept = caught.message;
	});
	check("해제 실패가 원래 실패를 덮지 않는다", kept === "본문 실패");
}
