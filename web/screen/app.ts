const root = document.documentElement;

export function setLive(state: "open" | "closed" | "connecting"): void {
	root.dataset.live = state;
}

document.addEventListener("htmx:ws:before:connection", () => setLive("connecting"));
document.addEventListener("htmx:ws:after:connection", () => setLive("open"));
document.addEventListener("htmx:ws:close", () => setLive("closed"));

export async function copyFrom(target: EventTarget | null): Promise<boolean> {
	if (!(target instanceof Element)) return false;
	const button = target.closest<HTMLElement>("[data-copy]");
	if (button === null) return false;
	const text = button.dataset.copy ?? "";
	try {
		await navigator.clipboard.writeText(text);
	} catch {
		return false;
	}
	const before = button.textContent;
	button.textContent = "copied";
	setTimeout(() => {
		button.textContent = before;
	}, 1200);
	return true;
}

document.addEventListener("click", (event) => {
	void copyFrom(event.target);
});
