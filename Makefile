.PHONY: run
run:
	@go run ./...

.PHONY: fmt
fmt:
	@echo "+ $@"
	@gofmt -l -w $(shell find . -type f -name '*.go' -not -path "./vendor/*")
