export default async function (): Promise<void> {
	void Promise.reject(new Error("dangling rejection"));
}
