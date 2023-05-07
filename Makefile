APP_NAME = "discobot-5023"

start:
	flyctl scale count 1

stop:
	flyctl scale count 0

restart:
	flyctl apps restart $(APP_NAME)

status:
	fly status

logs:
	fly logs

deploy:
	fly deploy

build:
	mkdir -p build
	GOOS=linux go build -o build/discobot discobot
