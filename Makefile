SQLC_VERSION := v1.30.0

.PHONY: sqlc-generate sqlc-check test-integration

sqlc-generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

sqlc-check:
	$(MAKE) sqlc-generate
	git diff --exit-code -- internal/storage/postgres/dbsqlc

test-integration:
	go test ./internal/storage/postgres -count=1
