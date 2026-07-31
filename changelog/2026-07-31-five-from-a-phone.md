# 2026-07-31 — Five defects reported from a phone, and the one that was the library's

Five screenshots, four causes in this tree and one upstream. Every number below was measured
in Chromium against the built bundle before and after, not estimated.

## 1 · A four-column table 717px tall, set four characters to a line

Reported as "font-size nhỏ lại trong table output và baseline vertical dọc đồng bộ align trên
cùng". Rendering "Resource" as `Reso / urce` is not a width problem, and it took measuring at
390 to see that it was three problems:

| cause | why it was there | what it does to a cell |
|---|---|---|
| `overflow-wrap: anywhere` | `.q, .prose, .source` in `styles.css` — a pasted path must never widen the page | inherits into every `td` and breaks words mid-character |
| `vertical-align: middle` | the browser's default for a `td` | a one-line cell floats in the middle of a 303px neighbour, so no two values in a row share a baseline |
| `inline-size: 100%` | the library's `.table` | a *preferred* width, so the table squeezes to the 317px card instead of taking what it needs and scrolling |

The first one is the interesting one, because the rule is right and the place is wrong. A cell
is the only construct in `.prose` that already **has** a horizontal escape — `.table-wrap` is
`overflow-x: auto` — so the rule that rescues a paragraph is what shreds a table. `normal`
inside `.table` is the same decision made where the escape exists, not a relaxation of it.

Measured at 390 · 1440, before → after:

| | 390 | 1440 |
|---|---|---|
| table height | 717px → **158px** | 186px → **139px** |
| tallest cell | 303px → **54px** | 49px → 36px |
| `scrollWidth === innerWidth` | yes → **yes** | yes → **yes** |

Three candidates were measured, not one. Cells at their natural width with a 16rem cap
(`max-content` + `min-inline-size: 100%`) beat both a plain de-shred (321px) and a 7rem column
floor (230px), and at 1440 all three are identical — `min-inline-size` wins there, so the
desktop layout is untouched. That is rule 28's order: decided at 390, then checked that it
survives widening.

The other half of the noise is what the model writes — `✅ Chốt` and sentence-long cells — so
the enumerate rule in `internal/rag/rag.go` gained a sentence. **The first version of that
sentence was backwards and is retired**: it read "no emoji in a cell, the status in the
documents' own words", because the request "thay thế emoji/icons nhiều để gọn hơn" was read as
*remove* the emoji when it asked to use a glyph *in place of* the noisy word. `✅ Chốt` is not
too much emoji, it is a glyph with a redundant word beside it.

What is there now compresses instead: a column whose values repeat gets one glyph per cell and
the table gets a one-line legend under it mapping each glyph to the documents' own term. That
keeps the rule the diagram bullet already states — a status the documents name is not silently
replaced — while the column itself goes from a word to a character, which is what was asked
for. An identifier is still never a glyph.

### 1b · the same card, two reports later: the width it never used, and three kinds of ragged

Reported as "tại sao không tận dụng WIDTH" and "tránh lệch so le". Four causes, and the first
is the one that makes the card look wrong at every width above a phone.

`.a > .clarify` sets `align-items: start`, which is correct for the ASK THIS button — the
comment beside it says so, and measures the 70px it protects — and wrong for the option group,
the one child that wants the whole card. Left to shrink-to-fit, the fieldset sized to its own
max-content: **520px inside 901px of card** at a 1000px window, so the longest option decided
the width and every shorter row trailed up to **263px** of dead space after its number.
`align-self: stretch` on that one child fixes it without touching the parent's value.

Then three separate kinds of ragged, which a reader sees as one mess:

| what | measured before |
|---|---|
| `.check` is the library's `align-items: center` — right for one line, wrong when it wraps | the number sat 15 · 26 · 38px from the top of its own row |
| the text has no `flex`, so everything after it started wherever the text stopped | right gaps 141 · 22 · 263 · 263 |
| the star is optional, and it came *after* the number | a row without one ended 22px further right |

`flex: 1` + `min-inline-size: 0` on the text, and `order: 1` on the number so the **number is
the column** and the star floats inside the row where its absence costs nothing.

The clamp is the rest: two lines, so every row is one height. Two and not one because at 390 a
one-line clamp truncated **all four** options — and these are the choices a reader is picking
between, while a `title` is a hover and a phone has none. Two truncated two of four and left
every option readable enough to choose from. `title` carries the whole wording either way, set
unconditionally: whether a row actually truncated is a measurement only the browser has, and a
branch in the component is logic rule 11 keeps out.

Measured after, at 390 · 1000: row heights uniform (46 · 24), the number column and the row's
right edge agree to **0px** at both, one left edge for every label, ASK THIS still 70px, and
`documentElement.scrollWidth === innerWidth`.

## 2 · ASK THIS asked the question that was already on screen

`composeClarify` prefixed a `[!QUESTION]` pick with the card's own legend, so the new turn's
heading repeated, word for word, the sentence still visible two blocks above it:

    Bạn muốn biết về phần nào của cú pháp Go? Cú pháp cơ bản

The reasoning behind the prefix was sound — a reading means nothing without the question it is
a reading *of* — and it was still wrong twice over. It reads as a duplicate, and half the query
is the model asking something, which is wording that appears in no document. The picks alone
now, for both kinds; the option is the wording a reader would recognise, which is exactly what
the prompt asks the model to put there. The function lost its `clarify` parameter with the
branch.

## 3 · `[!WARNING]` rendered as text, inside the panel it was asking for

`dressAlerts` reads the marker off the *first* text node of the first paragraph. A model that
writes two caveats writes them as two lines of one quote, marked folds those into one
blockquote and one paragraph, and `breaks: true` joins them with a `<br>` — so the second
marker is a later text node the loop has already walked past. `dropRepeats` strips a marker
that opens a line after a `<br>`; a sentence *about* `[!WARNING]` is prose and is left alone.

While there: `.callout` in 0.15.0 is a border and a tint and nothing else, so WARNING and
CAUTION are two oranges to anyone who has not learned the palette, and one colour to a
colour-blind reader. Each kind now opens with one glyph — ⚠️ 📝 💡 ❗ 🛑 — prepended into the
first paragraph, not the panel, because a text node before a block element gets a line of its
own. A `<b>` label was the other candidate and lost: it is a word to translate on every panel,
and the recipe has no icon slot.

## 4 · The import bar never filled, and the docs had said so all along

**8bit-nes 0.15.0 registers `@property --fill { syntax: "<percentage>"; inherits: false;
initial-value: 0% }`**, while the recipe's own CSS writes `.pbar { --fill: 0% }` and its `<i>`
reads `inline-size: var(--fill)`. Follow the container — which is what that CSS implies, and
what `ImportPanel.vue` did — and the child resolves the registered *initial* instead. Measured
at the same 66.66%: **0px on the container, 1031.89px on the `<i>`.**

The published docs put it on the `<i>`: `<span class="pbar"><i style="--fill:64%"></i></span>`.
So this was **ours**, not the library's — `AGENTS.md` says to read the pinned docs and this
markup was derived from the stylesheet instead. What the library owes is one line:
`.pbar { --fill: 0% }` can be read by nothing, and it is what made the wrong reading look like
the API.

So the bar was empty at every count, at every width, with the correct number in the sentence
beside it and no error anywhere. Worth naming why it survived: `changelog/2026-07-27` records
this exact markup verified by CDP with `--fill` moving 0% → 66.7%, and that verification read
the custom property off the `.pbar`, which is still 66.66% today. The bar was never measured.
The probe that catches it is one line — `getComputedStyle(bar.querySelector('i')).inlineSize`.

One attribute moved. The trap is in `CLAUDE.md` under *Traps that have already cost time*,
because it generalises: a registered custom property is the one case where "set it on the
parent" is not a style preference.

## 5 · Two lookalike boxes, in the wrong order, one of them clickable

A reply held both a `[!QUESTION]` and a `[!NEXT]`. `stripClarify` took the first match, so the
question was lifted into the card — which `ChatTurn.vue` draws **under** the prose — and the
offer stayed behind, rendering at the *top* as an inert `.callout info` with a `.tasklist`
nobody can tick. The model's order, inverted, with the interactive one last.

The prompt already forbids the shape (`rag.go`: "write nothing after that checklist"), so this
is a model ignoring it and the client rendering that mistake at its worst. `stripClarify` now
lifts **every** clarify block out of the prose and returns one: a `[!QUESTION]` if there is
one, because a reply that asks has nothing to offer next. Either way nothing is left in the
prose pretending to be pickable.

## Verifying these

`answer.js` needs a DOM, and rule 21 refuses a rig for one thing. What replaces it is the dev
server that already exists: `npx vite --port 5179` inside `web/ui`, a throwaway page importing
`/src/lib/answer.js`, and `pinchtab eval` on the result. Six cases, all six checked: the two
`[!WARNING]` lines, a reply holding both clarify blocks, `composeClarify`, a single `[!NOTE]`,
a `[!NEXT]` alone, and a paragraph that merely mentions `` `[!WARNING]` `` in backticks.

The two CSS defects were measured the same way, against `web/dist`'s own stylesheet in a
390px-wide iframe — which is how three table candidates got compared at two widths without a
build between them.
