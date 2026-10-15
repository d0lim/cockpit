function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function throws(fn: () => unknown): boolean {
	try {
		fn();
		return false;
	} catch {
		return true;
	}
}

export default function (): void {
	check("btoa", btoa("hello") === "aGVsbG8=");
	check("atob", atob("aGVsbG8=") === "hello");
	check("패딩 없는 입력과 공백을 받는다", atob("aGVs bG8") === "hello");
	check("Latin-1 왕복", atob(btoa("ÿ\u0000")) === "ÿ\u0000" && btoa("ÿ") === "/w==");
	check("Latin-1 밖 문자는 던진다", throws(() => btoa("가")));
	check("base64 가 아닌 입력은 던진다", throws(() => atob("a")) && throws(() => atob("a*bc")));
}
