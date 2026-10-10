
SHELL=/bin/bash


.PHONY: install
install:
	go install go.osspkg.com/goppy/v3/cmd/goppy@latest
	goppy setup-lib

.PHONY: lint
lint:
	goppy lint

.PHONY: license
license:
	goppy license

.PHONY: build
build:
	goppy build --arch=amd64

.PHONY: tests
tests:
	goppy test
	go test -bench=. -benchmem -count=3 ./...
	go test -fuzz=FuzzConverterDoesNotPanic -fuzztime=10s .
	go test -fuzz=FuzzCustomRuleProgressIsBounded -fuzztime=10s .

.PHONY: pre-commit
pre-commit: license lint tests build

.PHONY: ci
ci: install pre-commit

