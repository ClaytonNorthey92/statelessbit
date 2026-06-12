test:
	go fmt ./... && go test -count=1 -timeout=15s -v ./... && cd database && go test -v -count=1 ./...

migrate-create:
	migrate create -ext sql -dir database/migrations -seq $(MIGRATION_NAME)
