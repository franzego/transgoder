proto:
	protoc -I grpc/protos/transcoder \
    --go_out=./grpc/server --go_opt=paths=source_relative \
    --go-grpc_out=./grpc/server --go-grpc_opt=paths=source_relative \
    grpc/protos/transcoder/*.proto
run:
	go run main.go

build:
	go build -o transcoder main.go

vet:
	go vet ./...

test:
	go test ./...

test-v:
	go test -v ./...

race:
	go test -race ./...

tidy:
	go mod tidy