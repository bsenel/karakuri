# Karakuri

LLM agents for workflow automation and collaboration — orchestrating multi-role agent pipelines through Strategy, Discovery, Delivery, and Autonomous modes.

## Quick start

```bash
make build
./bin/server   # API at http://localhost:8080
./bin/krk strategy --idea "Build a task management API"
```

## Architecture

- **Thin CLI (`krk`)** — HTTP client only; all logic in the API server
- **Dynamic orchestration** — meta-agent produces execution plans from workflow YAML hints
- **VFS artifacts** — content-addressed blobs in SQLite
- **Git worktrees** — isolated delivery implementation contexts
- **No-op adapters** — system runs fully without external integrations

See [docs/architecture.md](docs/architecture.md) and [docs/openapi.yaml](docs/openapi.yaml).

## Adapter Ecosystem

| Category | Adapter | Interface | Suggested Library | Status |
|---|---|---|---|---|
| Version Control | GitHub | VersionControlAdapter | google/go-github | Planned |
| Version Control | GitLab | VersionControlAdapter | xanzy/go-gitlab | Planned |
| Project Management | Linear | ProjectManagementAdapter | linear-go | Planned |
| Project Management | Jira | ProjectManagementAdapter | go-jira | Planned |
| Design | Figma | DesignAdapter | REST client | Planned |
| Testing | Playwright | TestingAdapter | go-playwright | Planned |
| Testing | Go test runner | TestingAdapter | stdlib | Planned |
| Messaging | Slack | MessagingAdapter | slack-go/slack | Planned |
| Observability (external) | OpenTelemetry collector | ObservabilityAdapter | go.opentelemetry.io/otel | Planned |
| Observability (external) | Datadog | ObservabilityAdapter | datadog-go | Planned |
| OTel Exporter | AWS | Exporter | aws-sdk-go-v2 | Planned |
| OTel Exporter | Datadog | Exporter | datadog-go | Planned |
| LLM | Claude | ProviderAdapter | langchaingo | **Active (v1)** |
| LLM | Gemini, Cursor, Copilot | ProviderAdapter | langchaingo | Stub |
| Executor | Local | Executor | goroutines | **Active (v1)** |
| Executor | Celery, Restate | Executor | — | Stub |
| Storage | SQLite | StorageAdapter | GORM | **Active (v1)** |
| Storage | PostgreSQL, MySQL | StorageAdapter | GORM | Stub |

## Configuration

Copy and edit `config/default.yaml`. Set `ANTHROPIC_API_KEY` for live Claude responses (mock fallback when unset).

## CLI commands

```
krk strategy --idea "<concept>"
krk discovery --from-strategy <sha>
krk delivery --from-discovery <sha>
krk auto --validate --interval 1h
krk promote --from-research <sha> --via strategy
krk status <session-sha>
krk artifacts <session-sha>
krk resolve <session-sha> <checkpoint-id> <decision>
krk history --mode strategy
krk diff <sha-a> <sha-b>
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
