---
name: grammar-updates
description: Bring a vendored tree-sitter grammar to a new upstream revision without destroying this fork's local patches. Use before running _automation, when changing a pin in _automation/grammars.json, or when an update run reports that it skipped a grammar.
user-invocable: true
---

# Updating a vendored grammar

This fork carries local grammar patches upstream does not have. `_automation`
fetches upstream sources and overwrites the vendored copies verbatim, so the
normal update flow and a patched grammar are mutually exclusive. Establish which
kind you have before running anything:

```bash
for f in */parser.c; do
  head -c 8192 "$f" | grep -q "Locally patched" && echo "$f"
done
```

That is the same check the updater's guard makes. At the time of writing it
reports `csharp/parser.c` and `php/parser.c`; trust the command rather than that
sentence, since a patch can be retired.

## Unpatched grammar: use the updater

The README flow applies unchanged.

```bash
go run _automation/main.go check-updates          # reports only, writes nothing
go run _automation/main.go update <grammar-name>
go run _automation/main.go update-all
```

`_automation/grammars.json` decides what "newer" means per grammar.
`updateBasedOn: "tag"` follows releases. `updateBasedOn: "commit"` follows the
branch named in `reference` and moves on any upstream commit, so such a grammar
goes out of date far more often than a tag-tracked one. csharp tracks `master`
by commit.

## Patched grammar: regenerate by hand

Never run `update` or `update-all` against a patched grammar to bring it
forward. The updater has no notion of a patch: left to itself it replaces the
vendored sources with upstream content and drops every local divergence, with no
diff.

Rebase the patches onto the new upstream revision, regenerate, and replace the
vendored files by hand instead. The `grammar-disambiguation` skill carries that
procedure, including the byte-for-byte check that proves the patch set is
complete and the local CLI reproduces the committed tables before you change
anything. Do not skip that check: regenerating on top of an unverified baseline
makes every later diff untrustworthy.

The "Locally patched" header block at the top of `<lang>/parser.c` is the
authoritative record of what has to survive, and is detailed enough to re-derive
each patch. Unified `grammar.js` diffs under `analysis/` are a convenience only:
`/analysis/` is gitignored, so they are absent from a fresh clone.

## What the guard does if you run it anyway

`downloadGrammar` walks the whole `<lang>/` directory before writing anything. If
any file's first 8192 bytes contain the marker, it logs at `Warn` and skips that
grammar entirely. The scan covers the subdirectories that the php and typescript
downloaders write, which reconstructing each downloader's write targets would
miss.

A skipped grammar keeps its existing pin. Advancing the pin for files that were
never replaced would make `grammars.json` claim a revision the vendored sources
do not match, which is worse than being visibly out of date.

`update-all` therefore still completes and still updates every other grammar,
rather than aborting partway and leaving the grammars that did download recorded
at their old revisions.

`downloadFile` carries a last-resort backstop for a downloader that writes
outside the directory that was scanned. It refuses that one write, logs at
`Error`, and fails the run once `grammars.json` has been written. Reaching it
means a bug in that downloader rather than a routine skip.

`update <lang>` exits non-zero when it skips, so a skip is not mistaken for a
completed update. `update-all` still exits 0, because csharp and php are patched
permanently and failing on them would make a non-zero exit its steady state:
read its log for the skip warnings rather than its exit status.

## ALLOW_OVERWRITE_PATCHED

`ALLOW_OVERWRITE_PATCHED=1` disables the guard for one run. It exists for the
step after the patches have been re-applied by hand, when replacing the vendored
files is the intent. It is not a way to quiet the warning: setting it without
having re-applied the patches discards them exactly as if the guard were absent.

Only the exact value `1` enables it. `ALLOW_OVERWRITE_PATCHED=0` and
`ALLOW_OVERWRITE_PATCHED=false` leave the guard on, so the ordinary shell habit
of setting a flag to a falsey value cannot silently turn the protection off.

## After any update

Run the grammar's Go binding tests. For a patched grammar also run the upstream
corpus with `tree-sitter test`, expecting the full count to pass rather than
merely no new failures, and confirm every patch in the header block still
applies. A regenerated table can silently drop one. Update the header block if
the patch set changed.
