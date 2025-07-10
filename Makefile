BINARY_NAME=app

all: test build

build:
	go build -o ${BINARY_NAME} main.go aws.go web.go logging_middleware.go

test:
	go test -v --cover

run:
	go build -o ${BINARY_NAME} main.go aws.go web.go logging_middleware.go
	./${BINARY_NAME}

clean:
	go clean
	rm ${BINARY_NAME}
