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