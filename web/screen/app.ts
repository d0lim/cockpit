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

const boxEdge = 48;

function centerInBox(here: Element, box: HTMLElement): void {
	const frame = box.getBoundingClientRect();
	const at = here.getBoundingClientRect();
	if (at.top >= frame.top + boxEdge && at.bottom <= frame.bottom - boxEdge) return;
	const top = at.top - frame.top + box.scrollTop;
	box.scrollTop = Math.max(0, top - (box.clientHeight - at.height) / 2);
}

export function reveal(here: Element): void {
	const box = here.closest<HTMLElement>("[data-scroll-box]");
	if (box !== null) centerInBox(here, box);
	const head = document.querySelector<HTMLElement>("[data-head]")?.getBoundingClientRect().bottom ?? 0;
	const at = here.getBoundingClientRect();
	if (at.top < head || at.bottom > window.innerHeight) here.scrollIntoView({ block: "nearest" });
}

let shown = new Set<string>();

export function scrollToHere(): void {
	const keys = new Set<string>();
	for (const here of document.querySelectorAll<HTMLElement>("[data-scroll-here]")) {
		const key = here.dataset.scrollHere ?? "";
		keys.add(key);
		if (!shown.has(key)) reveal(here);
	}
	shown = keys;
}

export function measureHead(): void {
	const head = document.querySelector<HTMLElement>("[data-head]");
	root.style.setProperty("--head-height", `${head?.offsetHeight ?? 0}px`);
}

const kept = new Map<string, { top: number; left: number }>();

function namedBoxes(): HTMLElement[] {
	return [...document.querySelectorAll<HTMLElement>("[data-scroll-box]")].filter((box) => box.dataset.scrollBox !== "");
}

export function keepScroll(): void {
	kept.clear();
	for (const box of namedBoxes()) kept.set(box.dataset.scrollBox ?? "", { top: box.scrollTop, left: box.scrollLeft });
}

export function restoreScroll(): void {
	for (const box of namedBoxes()) {
		const at = kept.get(box.dataset.scrollBox ?? "");
		if (at === undefined) continue;
		box.scrollTop = at.top;
		box.scrollLeft = at.left;
	}
	kept.clear();
}

function settle(): void {
	measureHead();
	restoreScroll();
	scrollToHere();
}

document.addEventListener("DOMContentLoaded", settle);
document.addEventListener("htmx:before:swap", keepScroll);
document.addEventListener("htmx:after:settle", settle);
window.addEventListener("resize", measureHead);

const picks = "a, button, input, select, textarea, summary, label";

function panelClose(): HTMLAnchorElement | null {
	return document.querySelector<HTMLAnchorElement>("[data-panel-close]");
}

export function closeOnEscape(key: string): boolean {
	if (key !== "Escape") return false;
	const close = panelClose();
	if (close === null) return false;
	close.click();
	return true;
}

export function closeOnOutside(target: EventTarget | null): boolean {
	if (!(target instanceof Element)) return false;
	const close = panelClose();
	if (close === null) return false;
	if (target.closest("[data-panel]") !== null || target.closest(picks) !== null) return false;
	if (document.getSelection()?.isCollapsed === false) return false;
	close.click();
	return true;
}

export function followDiagramLink(event: MouseEvent): boolean {
	if (event.defaultPrevented || event.button !== 0) return false;
	if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return false;
	if (!(event.target instanceof Element)) return false;
	const link = event.target.closest("a");
	if (!(link instanceof SVGAElement)) return false;
	const follow = document.querySelector<HTMLAnchorElement>("a#follow");
	if (follow === null) return false;
	event.preventDefault();
	follow.href = link.href.baseVal;
	follow.click();
	return true;
}

document.addEventListener("click", (event) => {
	void copyFrom(event.target);
	if (followDiagramLink(event)) return;
	closeOnOutside(event.target);
});

document.addEventListener("keydown", (event) => {
	if (!event.defaultPrevented) closeOnEscape(event.key);
});
