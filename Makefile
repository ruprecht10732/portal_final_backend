SQLC=sqlc
GO=go

.PHONY: sqlc-generate sqlc-check build test

sqlc-generate:
	$(SQLC) generate

sqlc-check:
	$(SQLC) generate
	git diff --exit-code

build:
	$(GO) build ./...

test:
	$(GO) test ./...
