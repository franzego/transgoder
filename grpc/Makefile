proto:
	protoc -I grpc/protos/transcoder \
    --go_out=./grpc/server --go_opt=paths=source_relative \
    --go-grpc_out=./grpc/server --go-grpc_opt=paths=source_relative \
    grpc/protos/transcoder/*.proto