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
