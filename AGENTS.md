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

Use Go 1.27. Install `staticcheck` separately before running lint checks.

- `make build`: builds `bin/smtpbox` and `bin/sendmail`.
- `go run ./cmd/smtpbox`: starts local servers on HTTP 8080, SMTP 1025, and DNS 1053.
- `make test`: runs all packages with the race detector.
- `make cover`: writes race-enabled coverage results to `coverage.out`.
- `make fmt`: formats Go packages.
- `make lint`: runs `go vet` and `staticcheck`.
- `docker compose up --build`: builds and starts the container services.

## Coding style and naming conventions

Use standard Go formatting with tabs; run `make fmt` before submitting Go changes.
Keep package names lowercase. Use mixedCaps for private identifiers and MixedCaps for exported identifiers.
Follow existing constructors such as `NewServer` and `NewEmailService`.
Use `log/slog` for server logging. Keep protocol logic in its package and shared storage contracts in the root package.
Limit changes to the requested scope.

## Testing guidelines

Use Go's standard `testing` package, `*_test.go` files, and `TestName` functions.
Follow `TestSendEmail`: allocate a free local port, start a server, send mail, and inspect the backend.
Stop servers during cleanup. Keep automated tests independent of external mail services.
`TestSendSample` is skipped because it requires a running server.
No coverage percentage is enforced. CI runs `go build -v ./...` and `go test -v ./...`.

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
