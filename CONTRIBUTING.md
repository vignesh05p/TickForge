# Contributing to TickForge

Thank you for your interest in TickForge. This project is an **open-source backend reference** for real-time ingestion and aggregation—not a trading product.

## Contribution philosophy

- Prefer **small, focused PRs** with clear motivation.  
- Match **existing style** and package boundaries.  
- **Tests** for behavior that can regress; **docs** when behavior is user-visible.  
- **No scope creep:** reject features that belong in a trading platform or frontend before the core pipeline is stable.  

## Local setup

1. Install **Go 1.22+**.  
2. Clone the repository.  
3. Run `go mod download`, then `make test` (or `go test ./...`).  

When Docker Compose and PostgreSQL are wired up, this section will include connection strings and one-command bootstrap. Until then, the Phase 1 `cmd/server` binary runs the health/readiness HTTP skeleton, and `cmd/simulator` emits sample JSON ticks for local pipeline development.

## Branch naming

Use descriptive prefixes:

- `feat/` — new capability  
- `fix/` — bug fix  
- `docs/` — documentation only  
- `chore/` — tooling, modules, CI  
- `test/` — tests only  

Example: `feat/ingest-http-handler`, `docs/api-candles`.

## Commit messages

Use imperative mood and a prefix:

| Prefix | Use |
|--------|-----|
| `docs:` | Documentation |
| `chore:` | Tooling, CI, module bumps |
| `feat:` | New feature |
| `fix:` | Bug fix |
| `test:` | Tests |

Examples:

- `docs: add problem statement`  
- `chore: initialize go module`  
- `feat: add health endpoint`  
- `test: add candle aggregation tests`  
- `fix: handle invalid tick timestamp`  

## Pull request checklist

- [ ] Change is scoped and explained in the PR description.  
- [ ] `go test ./...` passes locally.  
- [ ] `go vet ./...` passes.  
- [ ] Code is formatted (`go fmt` or `make fmt`).  
- [ ] Public API or contract changes update `docs/API.md` (or linked docs).  
- [ ] No new dependencies without justification in the PR.  

## Testing expectations

- New logic should have **unit tests** where practical.  
- Table-driven tests are welcome for validation and aggregation.  
- Integration tests may be added when Dockerized dependencies exist; do not flake CI.  

## Formatting expectations

- Run `gofmt` on all Go files (`make fmt`).  
- Keep line length readable; avoid unrelated reformatting in the same PR as functional changes.  

## Code review expectations

- Reviewers may request **tests**, **docs**, or **simpler designs**.  
- **Nitpicks** on naming and structure are normal; prefer consistency over personal taste.  
- Maintainer merge when CI is green and concerns are addressed.  

## Welcome contributions

- Reliability: backpressure, shutdown, error handling  
- Observability: metrics, logging, health/readiness  
- Tests and benchmarks  
- Documentation: architecture notes, runbooks, API clarity  
- Developer experience: Makefile targets, Compose, migration hygiene  

## Not welcome (will be closed)

- **Trading advice**, **stock prediction**, or **fake AI signals**  
- **Frontend dashboards** ahead of backend stability (unless explicitly scoped by maintainers)  
- **Unnecessary frameworks** or heavy dependencies for problems solvable with stdlib  
- **Broker or exchange integration** in the core MVP path without a prior design agreement  
- Features that imply **financial advice** or **regulated** product positioning  

If you are unsure, open an issue describing the proposal before investing large effort.

## License

By contributing, you agree your contributions are licensed under the same license as the project ([LICENSE](LICENSE)).
