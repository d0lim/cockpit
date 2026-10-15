function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function pathOf(url: string): string {
	try {
		return new URL(url).pathname;
	} catch {
		return "unparsed";
	}
}

export default function (): void {
	let parsed = true;
	try {
		JSON.parse("{");
	} catch {
		parsed = false;
	}
	check("바인딩 없는 catch 가 예외를 삼킨다", !parsed);
	check("함수 안에서도 쓴다", pathOf("not a url") === "unparsed" && pathOf("http://h/p") === "/p");
}
