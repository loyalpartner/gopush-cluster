.PHONY: message comet web docker-build docker-up docker-down

message:
	@docker build -t message:latest -f docker/message ./

comet:
	@docker build -t comet:latest -f docker/comet ./

web:
	@docker build -t web:latest -f docker/web ./

docker-build:
	@docker-compose build

docker-up:
	@docker-compose up

docker-down:
	@docker-compose down
