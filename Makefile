APP_NAME=algo-saham
SERVICE_NAME=algo-saham

.PHONY: deploy pull build restart status logs

pull:
	git pull origin main

build:
	go mod tidy
	go build -o $(APP_NAME) .

restart:
	sudo systemctl restart $(SERVICE_NAME)

status:
	sudo systemctl status $(SERVICE_NAME)

logs:
	journalctl -u $(SERVICE_NAME) -f

deploy: pull build restart status