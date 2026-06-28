.PHONY: run test docker migrate clean

run:
	go run cmd/server/main.go -c configs/config.yaml

test:
	go test ./... -v

docker:
	docker-compose up -d

migrate:
	for f in scripts/migrations/*.sql; do mysql -u goim -pgoim123 goim < $$f; done

clean:
	docker-compose down -v
	rm -rf uploads/
