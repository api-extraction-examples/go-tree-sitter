---
name: grammar-disambiguation
description: Diagnose and fix ambiguities in tree-sitter grammars - keywords rejected where they should be valid identifiers, and operator nesting that binds the wrong way. Use when a grammar emits ERROR nodes for valid source, or produces a parse tree whose operator nesting contradicts the language spec.
user-invocable: true
---

# Tree-sitter grammar disambiguation

Two distinct failure classes, with different diagnostics and different fixes. Identify which one you have before reaching for a fix, because the remedies do not transfer.

| Symptom | Class | Section |
|---|---|---|
| A keyword is rejected where it should be a valid identifier | Keyword/identifier | [Class 1](#class-1-keywordidentifier-ambiguity) |
| Valid source produces ERROR, or parses with operator nesting that contradicts the spec | Precedence/recursion | [Class 2](#class-2-precedence-and-left-recursion-ambiguity) |

Use the locally installed CLI (`tree-sitter`), not `npx tree-sitter`. This repo's vendored tables must reproduce byte-for-byte, and `npx` may fetch a different CLI version that emits a different table layout. Check with `tree-sitter --version`.

---

## Class 1: keyword/identifier ambiguity

### When to use

When a grammar rejects a keyword in a position where it should be valid as an identifier: a function parameter name, a constant name, a named-argument label, a collection element.

### Diagnostic method

#### 1. Identify the full set of keywords that should be valid

Find the authoritative grammar specification for the language (for example `zend_language_parser.y` for PHP). Locate the rule governing identifier acceptance in the relevant context (`semi_reserved` for PHP named arguments). This is your ground truth.

#### 2. Test every keyword programmatically

```bash
for kw in keyword1 keyword2 ...; do
  echo '<test code using $kw>' > /tmp/test_${kw}.ext
  result=$(tree-sitter parse -p . /tmp/test_${kw}.ext 2>&1)
  if echo "$result" | grep -q "ERROR"; then
    echo "FAIL: $kw"
  else
    echo "PASS: $kw"
  fi
done
```

Do not guess which keywords fail; test all of them. The failures usually form a pattern that reveals the root cause.

#### 3. Classify the failures

Failing keywords almost always share a trait: they start **expression-level grammar rules**. In PHP:

- `clone` starts `clone_expression`
- `new` starts `object_creation_expression`
- `yield` starts `yield_expression`
- `print` starts `print_intrinsic`

Keywords appearing only in **statement-level** rules (`break`, `if`, `while`) typically work, because tree-sitter's `reserved('nothing', $.name)` mechanism handles them. They are not competing with an expression parse path in argument or expression contexts.

### Root cause

Tree-sitter's `word`/`reserved` mechanism resolves keyword-vs-identifier for most keywords. But when a keyword starts an expression-level rule and appears where expressions are expected, the parser takes the expression path rather than the identifier path. `reserved` alone cannot fix this, because the decision is made before the identifier interpretation is considered.

### Fix pattern

Add the failing keywords as explicit alternatives in the rule that accepts identifiers in that context.

PHP, in `_argument_name`:

```javascript
_argument_name: $ => seq(
  field('name', alias(
    choice(
      reserved('nothing', $.name),
      keyword('array', false),
      keyword('clone', false),   // added
      keyword('new', false),     // added
      keyword('yield', false),   // added
    ),
    $.name,
  )),
  ':',
),
```

C# has two rules serving this role:

- `_reserved_identifier`, the general keyword-as-bareword-identifier list. Upstream maintains this one; the local patch that used to extend it has been retired (see the precedence note below).
- `_argument_name_keyword` (Patch C), a surgical list for named-argument labels only, and still a local patch. `async` and `await` live here rather than in `_reserved_identifier`, because the upstream Async-Lambda corpus test depends on `async` reducing as a `(modifier)` in lambda position. Widening the general list would break it.

That split is the general lesson: prefer the narrowest rule that covers the failing context. A keyword valid as an argument label is not necessarily valid as a bareword everywhere.

### A precedence tiebreak may be needed alongside

Adding a keyword to the identifier list can create a new conflict with the rule that consumes it as a keyword. Upstream tree-sitter-c-sharp added `field`, `method`, `param`, `property`, `type`, and `typevar` to `_reserved_identifier` and bumped `attribute_target_specifier` to `prec(1)`, so the target-specifier reading wins when `:` follows and the identifier reading wins otherwise. This superseded our local Patch D, which had used a GLR conflict declaration for the same purpose.

Both approaches work. The precedence form is preferable when the two readings are distinguished by a single lookahead token, since it avoids widening the parse tables.

### Related: type-keyword ambiguity

A distinct but adjacent issue: keywords in `primitive_type` (`void`, `mixed`, `true`, `false`) that must also be valid identifiers, for example as constant names. Here `optional(field('type', $.type))` greedily consumes the keyword as a type token before the identifier rule fires. Add `$.primitive_type` as an explicit alternative in the identifier position:

```javascript
_const_element: $ => seq(
  alias(choice(reserved('nothing', $.name), $.primitive_type), $.name),
  '=',
  $.expression,
),
```

---

## Class 2: precedence and left-recursion ambiguity

### When to use

When valid source emits an ERROR node, or parses cleanly but with operator nesting that contradicts the language spec. The give-away is that the *fragment* parses in isolation while the *statement* does not, because the mis-nested node is not valid in its enclosing position.

Worked example: `*p++ = value;` in C#. `*p++` alone parsed fine. As an assignment target it did not, because the grammar built `(*p)++`, which is not an lvalue, so `= value` had nowhere to attach and became a sibling ERROR.

### Diagnostic method

#### 1. Dump the tree and compare nesting against the spec

```bash
tree-sitter parse -p . /tmp/repro.ext
```

Read the operator nesting and check it against the language's precedence table. The tree showed:

```
postfix_unary_expression        <- ++ applied outermost
  prefix_unary_expression       <- * applied first
    identifier p
ERROR                           <- = value, orphaned
```

C# binds postfix `++` tighter than prefix `*`, so `*p++` means `*(p++)`. The tree had it inverted. The ERROR is a *symptom* of the wrong nesting, not an independent problem, and fixing the nesting removes it.

#### 2. Establish what does and does not fail

Build a table of neighbouring constructs. This separates a narrow nesting bug from broad breakage, and becomes the regression suite. For the pointer case: `*p = 1`, `*(p + 1) = 1`, `*(int*)p = 1`, `int v = *p`, `f(*p++)`, and pointer arithmetic in conditions all parsed; only the increment-plus-assignment-target combination failed.

#### 3. Try a conflict declaration, and read the warning

Declare the two rules as conflicting and regenerate:

```javascript
conflicts: $ => [
  [$._pointer_indirection_expression, $.postfix_unary_expression],
],
```

The response is the key diagnostic:

- **No warning, behaviour changes.** A genuine GLR ambiguity. Resolve with `prec.dynamic` on the preferred alternative.
- **`unnecessary conflicts: ...`.** There is no conflict to resolve. Tree-sitter already decided statically, GLR will never explore the alternative, and **no amount of `prec`, `prec.left`, `prec.right`, `prec.dynamic`, or conflict declaration will redirect it.** Go to the fix below.

### Root cause of the static-resolution case

The competing rule is **left-recursive through a broad rule** such as `$.expression`. Both readings are complete parses of the same span, so the ambiguity is resolved when the tables are built rather than left to runtime.

In C#, `postfix_unary_expression` is `seq($.expression, choice('++', '--', '!'))`, and `expression` includes `lvalue_expression`, which includes the aliased pointer indirection. So `*p` can always reduce as a deref and then be consumed as the `$.expression` of an outer postfix. The parser never needs the alternative, so it never keeps it.

Precedence cannot arbitrate a decision that was never left open. This is the trap: raising precedence looks like the obvious fix and silently does nothing.

### Fix pattern

Give the parser a **structurally distinct, non-left-recursive production** to prefer, and alias it back so the public node type is unchanged.

```javascript
_pointer_indirection_expression: $ => choice(
  // Must outrank PREC.POSTFIX or the parser reduces `*p` first and then
  // applies `++` to the result, yielding the non-lvalue `(*p)++`.
  prec.right(PREC.POSTFIX + 1, seq(
    '*',
    alias($._postfix_incdec_operand, $.postfix_unary_expression),
  )),
  prec.right(PREC.UNARY, seq('*', choice(
    $.lvalue_expression,
    $.parenthesized_expression,
    $.cast_expression,
    $.prefix_unary_expression,
  ))),
),

// Dedicated and non-left-recursive: the shared postfix_unary_expression
// recurses through $.expression, which is exactly what lets `*p` complete
// as a deref and take `++` outside.
_postfix_incdec_operand: $ => prec(PREC.POSTFIX, seq(
  $.lvalue_expression,
  choice('++', '--'),
)),
```

Three properties make this work:

1. **Non-left-recursive.** The operand cannot swallow the enclosing expression, so the two readings are no longer the same span and the choice becomes real.
2. **A distinguishable LR item.** `* _postfix_incdec_operand` is its own production, so precedence has something to rank.
3. **Aliased.** Consumers still see `postfix_unary_expression`; no new node kind enters the public AST.

Note the asymmetry in that example. The prefix case (`*--p`) needed only `$.prefix_unary_expression` added to the operand choice, because prefix rules are not left-recursive. Only the postfix side needed a dedicated production. Do not assume the mirrored case needs the same treatment.

### Cost check

A dedicated production can inflate the parse tables. Compare before and after:

```bash
grep -E "#define (STATE_COUNT|LARGE_STATE_COUNT)" src/parser.c
```

The pointer-deref fix cost 12 states (13413 to 13425), which is negligible. A jump of thousands means the new rule is ambiguous against much more of the grammar than intended; narrow the operand.

### Watch for corrected nesting elsewhere

Fixing the nesting changes the tree for source that already parsed. `f(*p++)` previously nested as `(*p)++` and now nests as `*(p++)`. That is a fix, but it is still an AST change for working code: check the downstream converter's tests and any committed AST specs, and call it out when handing the change over.

---

## After fixing, either class

1. Regenerate: `tree-sitter generate`
2. Run the upstream corpus: `tree-sitter test` (expect the full count to pass, not merely "no new failures")
3. Re-run the class-1 keyword sweep, or the class-2 does/does-not-fail table, and confirm zero failures
4. Confirm every other local patch still works; a regenerated table can silently drop one
5. Add regression tests to the corpus
6. Update the "Locally patched" header block in the affected `<lang>/parser.c` so the next upstream upgrade can re-apply the patch

## Re-applying local patches on an upstream bump

The authoritative record of the patch set is the "Locally patched" header block at the top of `<lang>/parser.c`. That block is tracked, and it describes every local divergence in enough detail to re-derive it.

A unified `grammar.js` diff may also sit under `analysis/`, which is faster to apply. Treat it as a convenience, not a source of truth: `/analysis/` is listed in `.gitignore`, so that file is never committed and will be absent from a fresh clone.

Either way, reproduce the committed tables before changing anything:

```bash
git -C <upstream-clone> worktree add --detach /tmp/base <pinned-revision>
cd /tmp/base
# If the local patch file survives, apply it. Otherwise re-derive the
# patches from the parser.c header block and apply them by hand.
git apply <repo>/analysis/<lang>-<version>-*.patch
tree-sitter generate
tail -n +<header-lines+1> <repo>/<lang>/parser.c > /tmp/committed.c
tail -n +2 src/parser.c > /tmp/regen.c
diff /tmp/committed.c /tmp/regen.c   # expect no output
```

A byte-identical result proves three things at once: the patch set is complete, the header block describing it is accurate, and the local CLI version emits the same table layout. When the patch file is missing and the patches were reconstructed by hand, this check is what confirms the reconstruction is faithful, so do not skip it. If the diff is non-empty, reconcile before going further; regenerating on top of an unverified baseline makes every later diff untrustworthy.

Then rebase onto the new upstream revision with `git apply --3way` and resolve conflicts. Check whether upstream has absorbed any patch, as happened with Patch D, and retire rather than re-apply it. Regenerate the unified diff into `analysis/` afterwards so the next upgrade starts from a current one, and update the header block to match.
