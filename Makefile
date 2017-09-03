SERVICE_DIR = nexusd

.PHONY: all vet test service clean

all: vet test service

vet:
	go vet -all -composites=false -shadow=true ./...

test:
	go test -race ./...
	go test -race ./aat -websocket
	go test ./aat -websocket -msgpack

service: $(SERVICE_DIR)/nexusd

$(SERVICE_DIR)/nexusd:
	@cd $(SERVICE_DIR); go build
	@echo "===> built $(SERVICE_DIR)/nexusd"

clean:
	@rm -f $(SERVICE_DIR)/nexusd
	@rm -f $(SERVICE_DIR)/*.log
