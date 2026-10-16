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
	check("catch without a binding swallows the error", !parsed);
	check("works inside a function", pathOf("not a url") === "unparsed" && pathOf("http://h/p") === "/p");
}
