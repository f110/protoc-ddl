BAZEL ?= bazel
GO ?= $(BAZEL) run @rules_go//go --

.PHONY: sample/schema.sql
sample/schema.sql: sample/schema.proto
	$(BAZEL) run //sample:vendor_schema

.PHONY: sample/schema.entity.go
sample/schema.entity.go: sample/schema.proto
	$(BAZEL) run //sample:vendor_entity

.PHONY: sample/dao/schema.dao.go
sample/dao/schema.dao.go: sample/schema.proto
	$(BAZEL) run //sample/dao:vendor_dao

.PHONY: sample/dao/daotest/schema.mock.go
sample/dao/daotest/schema.mock.go: sample/schema.proto
	$(BAZEL) run //sample/dao/daotest:vendor_mock

.PHONY: gen-sample
gen-sample: sample/schema.sql sample/schema.entity.go sample/dao/schema.dao.go sample/dao/daotest/schema.mock.go

update-deps:
	$(GO) mod tidy
	$(GO) -- mod vendor
	find vendor -name BUILD.bazel -delete
	bazel run //:vendor_proto_source
	bazel run //:gazelle -- update

.PHONY: update-deps
