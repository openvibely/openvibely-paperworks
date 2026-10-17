.PHONY: deps test

deps:
	bash scripts/test-local.sh --deps-only

test: deps
	bash scripts/test.sh
