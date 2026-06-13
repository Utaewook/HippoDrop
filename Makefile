.PHONY: build install clean

BINARY_NAME=tardis
BUILD_DIR=build
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/tardis

build:
	@echo "Building Tardis..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/tardis/main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

install: build
	@echo "Installing to $(INSTALL_DIR)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@sudo chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installing default config to $(CONFIG_DIR)..."
	@sudo mkdir -p $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/tardis.yml ]; then \
		sudo cp tardis.example.yml $(CONFIG_DIR)/tardis.yml; \
		echo "Default config installed at $(CONFIG_DIR)/tardis.yml"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/tardis.yml, skipping."; \
	fi
	@echo "Install complete! You can now run 'tardis -c $(CONFIG_DIR)/tardis.yml'"

clean:
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete."
