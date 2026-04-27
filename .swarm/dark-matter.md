## Dark Matter: Hidden Couplings

Found 20 file pairs that frequently co-change but have no import relationship:

| File A | File B | NPMI | Co-Changes | Lift |
|--------|--------|------|------------|------|
| internal/leads/db/queries.sql.go | internal/leads/sql/queries.sql | 1.000 | 17 | 29.41 |
| agents/support/whatsapp_reply/SKILL.md | agents/support/whatsapp_reply/prompts/base.md | 1.000 | 4 | 125.00 |
| internal/auth/db/querier.go | internal/auth/db/queries.sql.go | 1.000 | 4 | 125.00 |
| internal/auth/db/querier.go | internal/auth/sql/queries.sql | 1.000 | 4 | 125.00 |
| internal/auth/db/queries.sql.go | internal/auth/sql/queries.sql | 1.000 | 4 | 125.00 |
| internal/whatsappagent/db/queries.sql.go | internal/whatsappagent/sql/queries.sql | 1.000 | 4 | 125.00 |
| internal/appointments/db/models.go | internal/exports/db/models.go | 1.000 | 37 | 13.51 |
| internal/appointments/db/models.go | internal/services/db/models.go | 1.000 | 37 | 13.51 |
| internal/auth/db/models.go | internal/catalog/db/models.go | 1.000 | 58 | 8.62 |
| internal/exports/db/models.go | internal/services/db/models.go | 1.000 | 37 | 13.51 |
| internal/identity/db/models.go | internal/imap/db/models.go | 1.000 | 37 | 13.51 |
| internal/identity/db/models.go | internal/notification/db/models.go | 1.000 | 37 | 13.51 |
| internal/identity/db/models.go | internal/quotes/db/models.go | 1.000 | 37 | 13.51 |
| internal/identity/db/models.go | internal/search/db/models.go | 1.000 | 37 | 13.51 |
| internal/identity/db/models.go | internal/webhook/db/models.go | 1.000 | 37 | 13.51 |
| internal/imap/db/models.go | internal/notification/db/models.go | 1.000 | 37 | 13.51 |
| internal/imap/db/models.go | internal/quotes/db/models.go | 1.000 | 37 | 13.51 |
| internal/imap/db/models.go | internal/search/db/models.go | 1.000 | 37 | 13.51 |
| internal/imap/db/models.go | internal/webhook/db/models.go | 1.000 | 37 | 13.51 |
| internal/notification/db/models.go | internal/quotes/db/models.go | 1.000 | 37 | 13.51 |

These pairs likely share an architectural concern invisible to static analysis.
Consider adding explicit documentation or extracting the shared concern.