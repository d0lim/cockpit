function down(depth: number): number {
	return down(depth + 1) + 1;
}

export default function (): number {
	return down(0);
}
