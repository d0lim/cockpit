import raw from "./labels.json";

type Table = Readonly<Record<string, string>>;
type Labels = Readonly<Record<"states" | "stages" | "wait" | "stop" | "destroy" | "verdicts" | "collectors" | "flowStatus" | "messages", Table>>;

export const LABELS: Labels = raw;

export const label = (table: keyof Labels, key: string | undefined | null): string => (key ? (LABELS[table][key] ?? key) : "-");

export const stageLabel = (phase: string | undefined | null): string => label("stages", phase);
