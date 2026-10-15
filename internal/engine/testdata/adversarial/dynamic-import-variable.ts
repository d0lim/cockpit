const target = ["./node", "builtin"].join("-");

export default async function (): Promise<unknown> {
	return import(target);
}
