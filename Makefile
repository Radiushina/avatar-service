api-build:
	docker run --rm -v ${PWD}/api/v1:/spec redocly/cli build-docs --config redocly.yml -o openapi.html openapi.yaml
