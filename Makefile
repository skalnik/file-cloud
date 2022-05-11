BINARY_NAME=app

all: test build

build:
	go build -o ${BINARY_NAME} main.go aws.go

test:
	go test -v

run:
	go build -o ${BINARY_NAME} main.go aws.go
	./${BINARY_NAME}

clean:
	go clean
	rm ${BINARY_NAME}
