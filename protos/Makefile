GO_OUT    := gen/go

.PHONY: proto
proto:
	@mkdir -p $(GO_OUT)
	protoc -I . \
		-I /opt/homebrew/include \
		--go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		auth/v1/auth.proto
