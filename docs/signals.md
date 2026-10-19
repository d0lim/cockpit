# Signals

This page holds the rules of the per-run signal store in `internal/signal`: where signals live, how the frozen segment and the tail work, what is dropped and how readers learn about it. The contract shapes are in `internal/contract/signal.go`.

## Layout

A run's signals live under `runs/<id>/signals/` in the hub state directory.

- `stream.jsonl`: one signal per line, append only. `seq` starts at 1 and increases by one per accepted signal inside the run.
- `segment.json`: whether the stream is frozen, the last frozen `seq`, the next `seq`, the drop counters carried over a compaction, and the read refusal.
- `kv/<collector>.json`: the state of one collector, written atomically and kept across hub restarts. The collector id must match `^[a-z0-9][a-z0-9-]{0,62}$`, and the state is at most 1 MiB.

The memory index is one 8-byte entry per `seq`: the line offset and the quota bucket, or a mark that the record was dropped or superseded.

## Redaction

Every signal passes the run masker at three points. On append, the string leaves and the map keys are masked, and attributes with credential names are hidden whole (the name check reads the original key). When two keys mask to the same text, both stay: keys the masker leaves unchanged keep their name, and the masked ones follow in the order of their original keys with `#2`, `#3` and so on. On read, and when the frozen segment is copied into the report bundle, every line is masked again with the current value list. A stream opens only with a masker, and an append without one is refused. When the report's rescan discards the bundle, the hub calls `RefuseReads`. The refusal is kept in `segment.json`, so reads of that run stay refused after a restart.

## Frozen segment and tail

When the report is finalized, `Freeze` copies every retained record up to that point into `<bundle>/signals/stream.jsonl`, and those records never change again. Later appends (resume, exec, periodic collection) go to the tail.

| Limit | Default | Applies to | Drop reason |
|---|---|---|---|
| Records per collector | 20000 | before and after the freeze, per window | `collector-quota` |
| Log lines per container | 10000 | before and after the freeze, per window | `container-quota` |
| Records in the tail | 50000 | after the freeze | `tail-cap` |

When a limit is exceeded, the oldest retained record of that bucket (or of the tail) is dropped, never the one just appended, and never a frozen record. The quotas restart at the freeze. Container log lines are events from the hub-owned collector `logs`, with the container name in the attribute `container`, and they count against their container's quota instead of a collector quota. The collector ids `runner` and `logs` are reserved for the hub.

## Reads

A read asks for the signals after a `seq`. The response carries whether the stream is frozen, the last frozen `seq`, and the drop counters by reason. When the next `seq` the reader expects was dropped, the response starts with a resync marker naming the dropped range, then continues with the next retained signals.

An event or span with an `id` that is emitted again from the same collector with the same kind is a revision; a span is a revision only within its own trace, so the same span id in another trace (an exec, for example) is a different span. The highest `rev` wins, and a tie goes to the later `seq`. Reads return only the winning revision, and the screen picks winners by the same key.

A revision that loses is superseded at once: the earlier winner when a new revision wins, or the new record itself when it arrives with a lower `rev`. A superseded record leaves its quota bucket (and the tail count), so a collector that re-emits the same rows on every tick holds one record per id, not one per tick. It is not data loss: it is not counted in `dropped`, it never starts a resync marker, and the frozen copy holds only the winners at the freeze. A frozen record is never superseded away. When a later revision of a frozen winner arrives, the frozen record stays in the stream file and the frozen copy, and reads return the new winner.

## Restart and compaction

On start, the index is rebuilt from `stream.jsonl`. An incomplete last line is cut off. A corrupt line before the last one fails the open instead of dropping data. The quotas and the tail cap are replayed in `seq` order, so the same records count as dropped as before the restart.

When more dropped or superseded records have accumulated since the last rewrite than there are retained records (and at least 4096), the file is rewritten with only the retained records. Each run of consecutive superseded `seq`s is kept as one tombstone line `{"superseded":[from,to]}`, so a restart marks them superseded again instead of reading them as a gap. The drop counters move into `segment.json`, and `seq` keeps increasing from where it was. Replay applies the revision rule in `seq` order, so a restart supersedes the same records as before.

## Runner spans

The run, flow and step spans come from the runner's result file, never from scripts. The run trace id is the run id, and the span ids are `run`, `flow:<flow>` and `step:<flow>/<step>`. A step that was still running when the runner stopped ends at the container exit time with status `incomplete`, and so do its flow and the run span. An exec is a new trace `<run id>/exec/<name>` whose root span `exec` links to the run span.
