SHELL := /bin/bash

DEMO_DIR := .demo
GATEWAY_URL := http://localhost:8080

.PHONY: test build demo-up demo-down demo-status demo-logs

test:
	go test ./...

build:
	go build ./...

demo-up:
	@mkdir -p $(DEMO_DIR)
	@if ls $(DEMO_DIR)/*.pid >/dev/null 2>&1; then \
		echo "Demo appears to be running. Run 'make demo-down' first."; \
		exit 1; \
	fi
	@echo "Starting mock upstreams..."
	@nohup go run ./cmd/mockupstream --port 3001 --name users > $(DEMO_DIR)/users.log 2>&1 & echo $$! > $(DEMO_DIR)/users.pid
	@nohup go run ./cmd/mockupstream --port 3002 --name orders > $(DEMO_DIR)/orders.log 2>&1 & echo $$! > $(DEMO_DIR)/orders.pid
	@nohup go run ./cmd/mockupstream --port 3003 --name products-a > $(DEMO_DIR)/products-a.log 2>&1 & echo $$! > $(DEMO_DIR)/products-a.pid
	@nohup go run ./cmd/mockupstream --port 3004 --name products-b > $(DEMO_DIR)/products-b.log 2>&1 & echo $$! > $(DEMO_DIR)/products-b.pid
	@nohup go run ./cmd/mockupstream --port 3005 --name legacy > $(DEMO_DIR)/legacy.log 2>&1 & echo $$! > $(DEMO_DIR)/legacy.pid
	@nohup go run ./cmd/mockupstream --port 3006 --name internal > $(DEMO_DIR)/internal.log 2>&1 & echo $$! > $(DEMO_DIR)/internal.pid
	@echo "Waiting for mock upstreams..."
	@for port in 3001 3002 3003 3004 3005 3006; do \
		for attempt in {1..30}; do \
			if curl -fsS "http://localhost:$$port/healthz" >/dev/null; then \
				break; \
			fi; \
			if [ "$$attempt" = "30" ]; then \
				echo "Mock upstream on port $$port did not become ready. See $(DEMO_DIR)/*.log"; \
				$(MAKE) --no-print-directory demo-down; \
				exit 1; \
			fi; \
			sleep 0.5; \
		done; \
	done
	@echo "Starting gateway..."
	@nohup go run ./cmd/gatewaykit --config gateway.yaml > $(DEMO_DIR)/gateway.log 2>&1 & echo $$! > $(DEMO_DIR)/gateway.pid
	@echo "Waiting for gateway..."
	@for attempt in {1..30}; do \
		if curl -fsS "$(GATEWAY_URL)/health" >/dev/null; then \
			echo "Demo is running at $(GATEWAY_URL)"; \
			echo "Logs are in $(DEMO_DIR)/"; \
			exit 0; \
		fi; \
		if [ "$$attempt" = "30" ]; then \
			echo "Gateway did not become ready. See $(DEMO_DIR)/gateway.log"; \
			$(MAKE) --no-print-directory demo-down; \
			exit 1; \
		fi; \
		sleep 0.5; \
	done

demo-down:
	@mkdir -p $(DEMO_DIR)
	@if ! ls $(DEMO_DIR)/*.pid >/dev/null 2>&1; then \
		echo "No demo services are recorded as running."; \
	else \
		for pidfile in $(DEMO_DIR)/*.pid; do \
			pid="$$(cat "$$pidfile")"; \
			name="$$(basename "$$pidfile" .pid)"; \
			if kill -0 "$$pid" >/dev/null 2>&1; then \
				echo "Stopping $$name ($$pid)"; \
				kill "$$pid" >/dev/null 2>&1 || true; \
			fi; \
			rm -f "$$pidfile"; \
		done; \
		echo "Demo stopped."; \
	fi

demo-status:
	@mkdir -p $(DEMO_DIR)
	@if ! ls $(DEMO_DIR)/*.pid >/dev/null 2>&1; then \
		echo "No demo services are recorded as running."; \
	else \
		for pidfile in $(DEMO_DIR)/*.pid; do \
			pid="$$(cat "$$pidfile")"; \
			name="$$(basename "$$pidfile" .pid)"; \
			if kill -0 "$$pid" >/dev/null 2>&1; then \
				echo "$$name running ($$pid)"; \
			else \
				echo "$$name not running (stale pid $$pid)"; \
			fi; \
		done; \
	fi

demo-logs:
	@mkdir -p $(DEMO_DIR)
	@if ! ls $(DEMO_DIR)/*.log >/dev/null 2>&1; then \
		echo "No demo logs found."; \
	else \
		tail -n 40 $(DEMO_DIR)/*.log; \
	fi
