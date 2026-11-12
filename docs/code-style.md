# Go style references

Repository-specific conventions live in [AGENTS.md](../AGENTS.md) under **Coding style and naming conventions**.
This page lists the external sources those rules come from, so a reviewer can
cite a link instead of re-arguing a point.

Read the tier 1 documents. Skim the rest as you need them.

## Tier 1: canonical

Treat these as the baseline. Every other guide assumes you have read them.

| Document | Why it matters here |
| --- | --- |
| [Effective Go](https://go.dev/doc/effective_go) | The shared baseline for naming, initialization, control flow, and concurrency. |
| [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) | 31 short rules, and the best per-line review checklist available. Link a section instead of writing the same comment twice. |
| [Go Doc Comments](https://go.dev/doc/comment) | The official comment format. It complements smtpbox's 2-to-3-line limit. |
| [Go Proverbs](https://go-proverbs.github.io/) | Rob Pike's aphorisms. "A little copying is better than a little dependency" is smtpbox's stdlib-preferred rule in one line. |
| [The Go Memory Model](https://go.dev/ref/mem) | Defines what a data race is. Run `make test` to check for data races against this specification. |

## Tier 2: Google Go style

Four documents, escalating from principles to specifics.

| Document | Contents |
| --- | --- |
| [Overview](https://google.github.io/styleguide/go/) | The five principles: clarity, simplicity, concision, maintainability, consistency. |
| [Style Guide](https://google.github.io/styleguide/go/guide) | The non-negotiable rules. |
| [Style Decisions](https://google.github.io/styleguide/go/decisions) | Naming, receivers, errors, and test structure. The most-cited of the four. |
| [Best Practices](https://google.github.io/styleguide/go/best-practices) | File organization and error-handling patterns. It also mandates the stdlib `testing` package with no assertion frameworks, which smtpbox follows. |

## Tier 2.5: modern Go idioms

The guides above say how to name things and how to shape a package. They do not
say which standard library call to reach for. That axis is version-dependent,
and it is where a stale habit survives longest.

| Document | Why it matters here |
| --- | --- |
| [JetBrains go-modern-guidelines](https://github.com/JetBrains/go-modern-guidelines) | 54 version-tagged rules mapping a pattern to its current idiom, Go 1.0 through 1.27. [`FEATURES.md`](https://github.com/JetBrains/go-modern-guidelines/blob/main/FEATURES.md) is the readable form, with a before-and-after for each. |
| [`modernize` analyzer](https://pkg.go.dev/golang.org/x/tools/gopls/internal/analysis/modernize) | The Go team's implementation of the same idea. Available through `golangci-lint`. |
| [`go fix`](https://go.dev/cmd/go#hdr-Update_packages_to_use_new_APIs) | Go 1.27 ships the modernizers in the toolchain, including `embedlit`, `errorsastype`, and `atomictypes`. `go fix -stringsbuilder=false ./...` applies them in place while preserving short string concatenations. |

`modernize` and `errorlint` cover many of these idioms.
The current `make lint` runs `go vet` and `staticcheck`.
Additional rules live in [AGENTS.md](../AGENTS.md) under **Coding style and naming conventions**.

### Rules smtpbox rejects

Cite this table instead of re-arguing the point in review.

| Rule | Why smtpbox declines |
| --- | --- |
| `stringsbuilder` | Rewrites short two- and three-iteration concatenation loops into `strings.Builder`, adding repeated `.String()` calls for no measurable gain. Avoid this rewrite for short loops. |
| `time.Tick` (Go 1.23) | The guideline relies on the ticker being collectable, but the loop it produces cannot be stopped. That contradicts smtpbox's rule that the starter of a goroutine stops it. Use `time.NewTicker`. |
| `cmp.Or` with calls | Sound as a rule, but every argument is evaluated before the call. Do not put a function call with side effects or real cost in one. |
| `encoding/json/v2` in existing code | Available on Go 1.27, and its stricter defaults are worth having in new code. Do not port `encoding/json` call sites wholesale: it encodes nil slices and maps as `[]` and `{}` rather than `null`, which changes API output. |

## Tier 3: Dave Cheney

| Document | Contents |
| --- | --- |
| [Practical Go (QCon Shanghai 2018)](https://dave.cheney.net/practical-go/presentations/qcon-china.html) | Identifiers, comments, package design, project structure, API design, error handling, concurrency. |
| [Practical Go (GopherCon Singapore 2019)](https://dave.cheney.net/practical-go/presentations/gophercon-singapore-2019.html) | The later revision of the same workshop. |
| [The Zen of Go](https://dave.cheney.net/2020/02/23/the-zen-of-go) | Ten values. "Before you start a goroutine, know how it will stop" is smtpbox's goroutine-lifetime rule. |
| [Don't just check errors, handle them gracefully](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully) | The origin of smtpbox's wrap-with-`%w`, no-sentinel-errors rule. |
| [SOLID Go Design](https://dave.cheney.net/2016/08/20/solid-go-design) | Interface segregation: accept interfaces, return structs. |
| [Functional options for friendly APIs](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) | Relevant to constructor options structs. |

## Tier 4: Mat Ryer

[How I write HTTP services in Go after 13 years](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/)
(2024) is the most directly applicable article on this page, because smtpbox is a
server. Read the whole thing. The table below maps its patterns to smtpbox's rule
for each.

| Pattern | smtpbox rule |
| --- | --- |
| One constructor takes every dependency and returns an `http.Handler`. | Adopt. Pass dependencies in one `Options` struct. |
| All routing lives in one place, so you can read the whole API surface at once. | Adopt. Add routes there, not in feature files. |
| `main()` is a few lines and calls `run(ctx, ...) error`, so startup is testable and errors surface once. | Adopt. |
| Bind listeners before serving, then shut down gracefully on a signal. | Adopt. A port conflict then fails as a named error before serving starts. |
| Handlers are closures over their dependencies, `func handleX(deps) http.Handler`, with per-handler setup done once outside the returned `HandlerFunc`. | Closures or methods on a handler type are both fine. The point worth keeping is that per-handler setup happens once at construction, not per request. |
| Request and response types are declared inside the handler, not shared package-wide, so one endpoint's shape cannot drift into another's. | Adopt. |
| A small `Validator` interface plus generic `encode` and `decode` helpers, instead of a framework. | Matches smtpbox's stdlib-preferred rule. |
| Tests start the real server and poll a readiness endpoint, so they exercise routing and middleware, not just the handler function. | Prefer `httptest.NewTestServer` over calling a handler directly when the route, method guard, or middleware is part of what you test. Its in-memory network also works inside a `synctest` bubble. |
| `sync.Once` for expensive lazy handler initialization. | Use `sync.OnceValue` when a handler needs costly setup that must not run at boot. |

## Tier 5: examples and other organization guides

| Document | Contents |
| --- | --- |
| [Go by Example](https://gobyexample.com) | Annotated snippets. A lookup reference, not a standard. |
| [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests) | A test-first walkthrough. Strong on table-driven tests. |
| [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) | The most concrete do-and-don't examples anywhere. **Caution:** it mandates `zap`, `testify`, and `multierr`, which conflict with smtpbox's stdlib-preferred and `log/slog` rules. Take the language sections and skip the dependency sections. |
| [Idiomatic Go Resources](https://dgryski.medium.com/idiomatic-go-resources-966535376dba) | Damian Gryski's annotated link collection. |
| [go-perfbook](https://github.com/dgryski/go-perfbook) | Performance work. Read it before you optimize, not instead of measuring. |

## Errors, testing, and security

| Document | Contents |
| --- | --- |
| [Errors are values](https://go.dev/blog/errors-are-values) | Why Go has no exceptions, and what to do instead. |
| [Working with errors in Go 1.13](https://go.dev/blog/go1.13-errors) | `errors.Is`, `errors.As`, and `%w`: the semantics smtpbox's no-sentinel rule depends on. |
| [Testing concurrent code with testing/synctest](https://go.dev/blog/synctest) | smtpbox is on Go 1.27. `synctest` makes timer-driven code deterministically testable. |
| [govulncheck](https://go.dev/blog/vuln) | Scans dependencies for known vulnerabilities. Run it before a release. |
