import raw from "./labels.json";

type Table = Record<string, string>;
type Labels = { stages: Table; wait: Table; stop: Table; destroy: Table; collectors: Table; flowStatus: Table };

export const LABELS = raw as Labels;

export const label = (table: keyof Labels, key: string | undefined | null): string => (key ? (LABELS[table][key] ?? key) : "-");

export const stageLabel = (phase: string | undefined | null): string => label("stages", phase);
