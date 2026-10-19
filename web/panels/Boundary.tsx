import { FROZEN_DIVIDER } from "../signals/model";

export const Divider = () => (
	<li className="divider" role="separator">
		{FROZEN_DIVIDER}
	</li>
);

export const Dropped = ({ text }: { text: string }) => <li className="dropped">{text}</li>;

export const DividerRow = ({ span }: { span: number }) => (
	<tr className="divider" role="separator">
		<td colSpan={span}>{FROZEN_DIVIDER}</td>
	</tr>
);

export const DroppedRow = ({ span, text }: { span: number; text: string }) => (
	<tr className="dropped">
		<td colSpan={span}>{text}</td>
	</tr>
);
