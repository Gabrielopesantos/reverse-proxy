.PHONY: gofmt
fmt:
	@echo "+ $@"
	@gofmt -l -d $(shell find . -type f -name '*.go' -not -path "./vendor/*")
