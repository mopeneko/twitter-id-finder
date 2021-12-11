.PHONY: build
build:
	go build -o main main.go

.PHONY: clean
clean:
	$(RM) main