# Output style

How the CLI talks: errors, warnings, success lines, prompts, and help text. Help *layout* (fragments, examples, wrapping) is enforced by `cmd/help_lint_test.go`; this page covers the words.

The voice is a terse, competent tool. It says what happened and what to do next, then stops.

## Principles

- **Lead with the fact.** The first words say what happened or what is wrong. Context comes after, and only if it changes what the reader does.
- **Give the next step as a command.** `run astro login`, not "you may need to authenticate".
- **One idea per sentence.** Split a sentence that needs an em dash, a semicolon chain, or a second "which".
- **Leave the reasoning in the code.** Why the CLI was designed a certain way belongs in a comment or in `docs/`. Output explains a rule only when the reader can't act correctly without it.
- **Plain words.** Write "use", "check", "can't". Avoid "utilize", "ensure", "leverage".

## Errors

```
could not reach the Airflow API at http://localhost:8080: connection refused
no Deployment named "etl" in Workspace analytics; run astro deployment list to see them
--output must be table or json, not "yaml"
```

- Start lowercase with no trailing period. The CLI prints `Error:` in front, and Go wraps errors with `: `.
- Use this order: what failed, then the cause if known, then the fix. Join the parts with `: ` or `;`, or start a new sentence.
- Name the exact object: `no Deployment named "etl" in Workspace analytics`, not `Deployment not found`.
- State the rule rather than blaming the user: `names use lowercase letters, numbers, and hyphens`, not `you entered an invalid name`.
- Say "cannot" for something blocked now and "could not" for an attempt that failed. Don't use "unable to", "failed to" (unless you're naming a failed step), "oops", "sorry", "an error occurred", or "something went wrong".
- Never end on "try again" alone. Give the command to run, or the command that shows the state when the outcome is unknown.
- Keep `%w` wrapping, sentinel errors, and anything code or tests match with `errors.Is` or `strings.Contains`.

## Warnings and notes

Use `cliout.Warn` / `cliout.Note` (`ansi.Warn` / `ansi.Note` outside `cmd/`). They add the label, so the message doesn't repeat it. Write one or two short sentences, starting with a capital letter.

## Success lines

Use the past tense, the object, then its name. Don't add "Successfully" or "was successfully".

```
Deleted Deployment etl
Added Organization API token ci-bot to Workspace analytics
Updated team data-eng to WORKSPACE_MEMBER
```

## Prompts and confirmations

Ask directly with the imperative or a short question. Don't say "please" or "you would like to".

A confirmation names the action, the exact object, and the consequence. Reuse that verb in the success line. Never ask a bare "Continue?", "Proceed?" or "Are you sure?".

```
Select the Organization API token to add to the Workspace
Delete Deployment etl? This cannot be undone. (y/N)
Hard-delete the Deployment? (after a line that says all its data, including the database, is lost)
```

## Counts and empty results

- Count with the noun that fits: `1 Dag`, `3 Dags`. Never write `Dag(s)`; `plural` in `cmd/local` does this.
- Zero is `No Deployments in Workspace analytics`, not `0 Deployments`. Follow it with the command that creates the first one when there is one.

## Help text

- `Short` is an imperative fragment (the lint enforces the shape).
- The first sentence of `Long` says what the command does. Add more paragraphs only for what a user needs in order to use it: inputs, defaults, side effects, and caveats.
- Don't defend the design ("this is the answer to…", "on purpose", "by design", "never…"). State the behavior.

## Words and patterns to avoid

These show up in machine-written prose and make output read as padded or evasive:

| Avoid | Use |
|---|---|
| em dash (`—`) to bolt on a clause | a colon, a semicolon, a period, or parentheses |
| "X, not Y" / "not A, not B" contrast, for rhetoric | state X. Keep "must be a or b, not %q" when it names the bad input |
| Successfully …, … was successfully … | the past-tense verb |
| Please, Sorry, Oops, `!` | nothing |
| Unable to | could not / cannot |
| genuinely, actually, simply, just, really, whole (as emphasis) | nothing |
| ensure, utilize, leverage, seamless, robust | make sure → name the action; use |
| in order to | to |
| Note that …, It's worth noting … | the fact itself, or `cliout.Note` |
| emoji, decorative symbols | nothing |
| "It's not X, it's Y", rhetorical questions, lists of three for rhythm | the plain statement |

## Terms

- Use one name per concept and one verb per operation. Don't switch to a synonym for variety.
- Capitalize **Deployment**, **Workspace**, and **Organization** when the text means the Astro or APC resource. Flag names keep their spelling (`--deployment`).
- **Dag**, as Airflow 3 spells it. Keep **DAG** only where it quotes Airflow 1/2 or an API field.
- Write **Airflow**, **Astro Runtime**, and **Astro CLI** with capitals. Write **pyproject.toml**, **uv**, and command lines as they are typed.

## What not to change

Rewording output must not change its contracts:

- `--output json` keys, values, and error kinds, and anything under a `testdata/schema` golden.
- Strings that code matches, whether ours (`strings.Contains(err.Error(), …)`) or another tool's output we parse.
- Telemetry event names, flag names, and command names.
