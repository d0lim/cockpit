def lines(title; items): if (items | length) > 0 then "\n**" + title + "**\n\n" + (items | map("- " + .) | join("\n")) + "\n" else "" end;
"## cockpit run: \(.verdict)\n\n"
+ "| | |\n| --- | --- |\n"
+ "| Suite | \(.suite) |\n"
+ "| Commit | `\(.commit[0:12])` |\n"
+ "| Seal | \(.seal), network \(.network // "sealed") |\n"
+ "| Steps | \(.steps.actual // "?") of \(.steps.expected // "?"), failed \(.steps.failed), skipped \(.steps.skipped) |\n"
+ "| Exit code | \($code) |\n"
+ (if .stoppedAt then "\n**Stopped at** \(.stoppedAt.flow) › \(.stoppedAt.step): \(.stoppedAt.message // "")\n" else "" end)
+ lines("Reasons"; .reasons)
+ lines("Not proven as declared"; .notProven.declared)
+ lines("Unread"; .notProven.unread)
