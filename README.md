# GoIM

GoIM is a Go, React, MySQL, Redis, and RabbitMQ instant-messaging application.

## Project layout

- `backend/` — Go API, WebSocket server, migrations, and tests
- `frontend/` — React/Vite web client
- `docker-compose.yaml` — local MySQL, Redis, and RabbitMQ dependencies
- `docs/` — cross-project documentation

## Local development

1. Copy the environment examples:

   ```powershell
   Copy-Item .env.example .env
   Copy-Item backend/configs/config.local.example.yaml backend/configs/config.local.yaml
   Copy-Item frontend/.env.example frontend/.env.local
   ```

   If Windows reserves port `13306`, set `MYSQL_PORT=3307` in `.env` and change `mysql.port` in `backend/configs/config.local.yaml` to `3307`.

2. Start infrastructure:

   ```powershell
   docker compose up -d
   ```

3. Start the backend:

   ```powershell
   Set-Location backend
   go run ./cmd/server -c configs/config.local.yaml
   ```

4. Start the frontend in another terminal:

   ```powershell
   Set-Location frontend
   npm ci
   npm run dev
   ```

Open `http://localhost:5173`. The API health check is at `http://localhost:18080/health`.

## Tests

```powershell
Set-Location backend; go test ./...
Set-Location frontend; npm test; npm run build
```

Backend integration tests expect the dependencies from the root Compose file to be running.

## Production image and GHCR

The production `Dockerfile` builds the frontend and backend into one image, which serves the SPA and API from the same origin. The GitHub Actions workflow publishes images to GHCR on pushes to `main` and version tags.

For a full production stack, copy `.env.production.example` to a protected file outside the repository, set all secrets, then run:

```sh
docker compose --env-file /path/to/goim.production.env -f docker-compose.prod.yaml up -d
```
