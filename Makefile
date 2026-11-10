migrate-sql:
ifneq "$(name)" ""
	migrate create -ext sql -dir migrations/postgres $(name)
else
	echo "\nSpecify migration script name\n";
endif

api-build:
	docker run --rm -v ${PWD}/api/v1:/spec redocly/cli build-docs --config redocly.yml -o openapi.html openapi.yaml

lint:
	golangci-lint run --config .golangci.yml

test-goroutines:
	go test -count=1 -timeout 90s \
		-run 'TestCollectRuntimeMetricsExits|TestListenGoroutineExits|TestWatchQueueDepthExits|TestRunGoroutinesExit' \
		./cmd/server/di/providers/ ./internal/broker/ ./internal/worker/