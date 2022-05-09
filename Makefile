BINARY_NAME=app

all: build

build:
	go build -o ${BINARY_NAME} main.go aws.go

run:
	go build -o ${BINARY_NAME} main.go aws.go
	./${BINARY_NAME}

clean:
	go clean
	rm ${BINARY_NAME}
