# Repository Guidelines

## Project structure and modules

`smtpbox` is a Go service for capturing development email and retrieving it through a REST API.

- `cmd/smtpbox/`: starts the SMTP, HTTP, and DNS servers.
- `cmd/sendmail/`: provides a sample SMTP client.
- `smtp/`, `http/`, and `dns/`: implement protocol servers and handlers.
- `inmem/`: stores mailboxes and messages in memory.
- `email.go` and `errors.go`: define shared models, the storage interface, and errors.
- `smtp/*_test.go`: contains SMTP integration tests.
- `.github/workflows/go.yml`: defines CI build and test checks.

## Build, test, and development commands

Use Go 1.27.4 and `golangci-lint` v2.15.0.
Install the tool with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`.

- `make build`: builds `bin/smtpbox` and `bin/sendmail`.
- `go run ./cmd/smtpbox`: starts local servers on HTTP 8080, SMTP 1025, and DNS 1053.
- `make test`: runs all packages with the race detector.
- `make cover`: writes race-enabled coverage results to `coverage.out`.
- `make fmt`: formats Go packages and groups imports with `goimports`.
- `make fmt-check`: checks formatting without changing files.
- `make lint`: runs the analyzers in `.golangci.yml`, including `go vet` and `staticcheck`.
- `docker compose up --build`: builds and starts the container services.

## Coding style and naming conventions

Before writing or reviewing Go code, read [Go style references](docs/code-style.md).

- Preserve existing public packages and shared backend contracts. Put implementation-only packages under `internal/`.
- Group imports: standard library, third-party dependencies, then `github.com/yavosh/smtpbox`.
- Use `log/slog` through a package-local `clog*` helper. Every log line includes a `component` attribute naming the subsystem.
- Keep comments to two or three lines. Explain the non-obvious behavior.
- Do not panic outside `main` or `init`.
- Limit changes to the requested scope.

## Testing guidelines

Use loopback port `0` and retain listeners until cleanup. Keep tests independent of external mail services.
No coverage percentage is enforced. CI builds all packages, runs race tests, and checks lint and formatting.

## Commit and pull request guidelines

Use existing prefixes: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, or `chore:`.
Write short, imperative subjects, such as `docs: add contributor guide`.
Work on a named feature branch; never push to `main`.
Describe behavior changes, link relevant issues, and report validation commands in pull requests.
Update `README.md` when flags, ports, or API responses change.

## Security and configuration

Configure ports and domains through flags or environment variables documented in `README.md`.
Never read or commit secret files, including `.env`, `.envrc`, `*.auto.tfvars`, or `credentials.json`.
Confirm before modifying shared or production state.
