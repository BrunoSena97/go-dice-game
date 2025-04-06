# Go-Dice-Game

![Gopher](/assets/gopher.jpg)

## Description

This project contains the backend implementation for a simple dice game, developed as part of a technical assessment. The backend is written in Go and uses WebSockets for real-time client communication. It manages player wallets using PostgreSQL and utilizes Redis for intermediate play state management (concurrency control). **Includes a basic SvelteKit frontend application, also containerized, providing a user interface for the game.**

The core game logic involves betting on whether the sum of two dice will be less than 7 ("lt7") or greater than 7 ("gt7"). A roll of exactly 7 results in a loss for the player. _(Note: This rule differs from the "even/odd" example initially described in the assessment PDF)._

## Project Structure

```
dice_game/
├── dice_game_backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── constants/
│   │   ├── game/
│   │   ├── handler/
│   │   ├── platform/
│   │   │   ├── database/
│   │   │   └── redis/
│   │   └── wallet/
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── dice_game_frontend/
│   ├── src/
│   ├── static/
│   ├── Dockerfile
│   ├── ecosystem.config.cjs # PM2 config for frontend container
│   ├── package.json
│   ├── svelte.config.js
│   └── ... (other frontend files)
├── db_init/
│   └── 01-init.sql
├── .env.example
├── .gitignore
├── docker-compose.yml
└── README.md
```

## Requirements

- Docker
- Docker Compose
- Go (version 1.20+ recommended, for local backend dev)
- Node.js / npm (for local frontend dev)
- Git

## Setup

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/BrunoSena97/go-dice-game.git
    cd go-dice-game
    ```
2.  **Configure Environment:**
    - Copy the example environment file:
      ```bash
      cp .env.example .env
      ```
    - Edit the `.env` file and provide actual values, especially for:
      - `DB_PASSWORD`: Your desired PostgreSQL password.
      - _(Optional)_ `DB_USER`, `DB_NAME`, `DB_PORT_HOST` if you want to change defaults.
      - _(Optional)_ `REDIS_PASSWORD` if you configure Redis with one.
      - _(Optional)_ `BACKEND_PORT_HOST` if you want to change the port the backend is exposed on locally.
      - _(Optional)_ `FRONTEND_PORT_HOST` if you want to change the port the frontend is exposed on locally.
      - _(Optional)_ `MAX_BET_AMOUNT` if you want to override the default max bet (250).
    - **Important:** The `.env` file is ignored by Git (`.gitignore`) and should **not** be committed.

## Running the Project

The easiest way to run the complete application (frontend, backend) and its dependencies (PostgreSQL, Redis) is using Docker Compose.

1.  **Build and Start Containers:**
    From the project root directory, run:

    ```bash
    docker compose up --build -d
    ```

    - `--build`: Ensures the frontend and backend images are built/rebuilt with the latest code.
    - `-d`: Runs the containers in the background.
    - This starts the `frontend`, `backend`, `db`, and `redis` services.

2.  **Accessing the Service:**

    - The **frontend UI** will be accessible at `http://localhost:4300` (or the port specified by `FRONTEND_PORT_HOST` in your `.env` file). **This is the primary way to interact with the game.**
    - The backend WebSocket server will be running and accessible at `ws://localhost:8080/ws` (or the port specified by `BACKEND_PORT_HOST` in your `.env` file). The frontend connects to this internally.
    - PostgreSQL is accessible on `localhost:5433` (or `DB_PORT_HOST`) for debugging if needed.
    - Redis is accessible on `localhost:6380` (or `REDIS_PORT_HOST`) for debugging if needed.

3.  **Stopping the Services:**

    ```bash
    docker compose down
    ```

    To also remove the persistent database volume (deletes all data):

    ```bash
    docker compose down -v
    ```

4.  **Local Development:**
    - **Full Stack (Recommended for interaction):** Use `docker compose up --build` (without `-d` to see logs). This runs everything containerized.
    - **Backend Only (for Go code iteration):**
      - Ensure DB and Redis are running: `docker compose up -d db redis`
      - Navigate to the backend directory: `cd dice_game_backend`
      - Run the Go server with the `-dev` flag: `go run ./cmd/server/main.go -dev`
      - _You will need a separate WebSocket client (like Postman or wscat) or the running frontend (dev server or container) to interact with the backend._
    - **Frontend Only (for UI code iteration):**
      - Ensure the backend (and its dependencies) are running, ex:, via `docker compose up -d backend db redis`.
      - Navigate to the frontend directory: `cd dice_game_frontend`
      - Install dependencies: `npm install`
      - Run the SvelteKit dev server: `npm run dev`
      - Access the UI via the URL provided by the dev server (usually `http://localhost:5173`). It will connect to the backend WebSocket specified in its code.

## API / WebSocket Protocol

Communication happens over a single WebSocket endpoint: `ws://<backend_host>:<backend_port>/ws`. Messages are JSON strings. The frontend handles this communication. For direct backend testing details:

- **Format:** See the message/payload struct definitions in `dice_game_backend/internal/handler/`. Includes base types `WsMessage` (Client->Server) and `ServerMessage` (Server->Client). See `internal/constants/constants.go` for message type strings.
- **Client Actions (`type`):**
  - `play`: Initiates a game round. Payload: `{"clientId": string, "betAmount": int64, "betType": string("lt7"|"gt7")}`.
  - `get_balance`: Requests current balance. Payload: `{"clientId": string}`.
  - `end_play`: Signals leaving the game; server sends final balance and closes connection. Payload: `{"clientId": string}`.
- **Server Messages (`type`):**
  - `play_result`: Result of a play round. Payload: `{"clientId": string, "die1": int, "die2": int, "outcome": string("win"|"lose"), "betAmount": int64, "winnings": int64}`. (Winnings = net amount won, 0 on loss).
  - `balance_update`: Provides current balance. Payload: `{"clientId": string, "balance": int64}`.
  - `play_ended`: Confirmation of `end_play`. Payload: `{"clientId": string, "finalBalance": int64}`.
  - `error`: Indicates an error occurred. Payload: `{"code": string, "message": string}`. (See `internal/constants/constants.go` for error codes).

## Testing

1.  **Via Frontend UI (Primary Method):**

    - Start the application using `docker compose up --build`.
    - Access the running frontend application in your browser at `http://localhost:4300` (adjust port if needed via `FRONTEND_PORT_HOST` in `.env`).
    - Use the UI's "Connect & Play Game" button, place bets using chips and bet type buttons, click "Play", observe results, and use "End Play".
    - Test error conditions via UI interactions (ex:, betting more than balance).

2.  **Direct Backend Testing (Optional):**

    - Use a tool capable of sending WebSocket messages (ex:, `wscat`, Postman) to connect directly to `ws://localhost:8080/ws` (adjust port if needed via `BACKEND_PORT_HOST`).
    - Send raw JSON messages according to the protocol defined above.
    - **Postman Collection:** A [Postman](https://ibrain-api-testing.postman.co/workspace/Dice-Game-ws~db32fe53-4afb-4179-be00-d232180314ea/collection/67f2e75994325a6917695160?action=share&creator=21969514) collection demonstrating example requests should accompany this project.

3.  **Example Flow (Manual WebSocket Client):**

    - Connect to the WebSocket endpoint.
    - Send `get_balance` (wallet created with 500 balance on first action if not existing). Example: `{"type":"get_balance", "payload":{"clientId":"manual_test_1"}}`
    - Send a `play` message. Example: `{"type":"play", "payload":{"clientId":"manual_test_1", "betAmount": 50, "betType": "lt7"}}`
    - Observe `play_result` and `balance_update` responses.
    - Send subsequent `play` messages.
    - Send an `end_play` message. Example: `{"type":"end_play", "payload":{"clientId":"manual_test_1"}}`
    - Test error conditions (insufficient funds, invalid bets, concurrent plays - expect "ACTIVE_PLAY_EXISTS" error).

4.  **Debugging:**
    - Frontend Logs: `docker compose logs -f frontend`
    - Backend Logs: `docker compose logs -f backend`
    - Redis: `docker compose exec redis redis-cli` (use `KEYS *`, `GET keyname`, `TTL keyname`)
    - Database: `docker compose exec db psql -U ${DB_USER} -d ${DB_NAME}` (use `SELECT * FROM wallets;`) (Requires values from `.env`)

## Assumptions & Deviations & Design Choices

- **ClientID Handling:** For simplicity in this assessment, the `ClientID` is generated randomly by the frontend on load and sent in message payloads. The backend currently trusts this ID. **A production system would require a secure authentication mechanism** (ex:, tokens via initial HTTP auth or ws message) to establish and validate the user's identity associated with a WebSocket connection.
- **`end_play` Workflow:** The implemented workflow **deviates** from the _example_ sequence shown in the assessment PDF. In the PDF example, `"play"` returns the result, and `"end_play"` credits the winnings. In _this implementation_, the `"play"` handler completes the entire round atomically: it debits the bet, determines the outcome (via `game.Service`), **credits any winnings immediately** using the `wallet.Service`, and then sends the results back. The `active_play` Redis key acts only as a short-lived lock (~15s expiry) to prevent _concurrent_ processing for the same client, and is deleted promptly after processing. The `"end_play"` message is now only used to retrieve the final balance and trigger a server-side disconnect; it does not credit winnings. This change was made to simplify the state management and create a more atomic play loop, while still preventing overlapping processing via the Redis lock.
- **Game Rules:** The game logic was implemented as "Sum of 2 Dice < 7 / > 7 / 7 loses" based on development discussions, differing from the "Even/Odd" example in the PDF.
- **RTP:** The payout for a win is 1:1 (meaning the player receives their stake back _plus_ an amount equal to their stake). With the current "<7 / >7 / 7 loses" rules on 2 dice, this results in an approximate Return To Player (RTP) of 83.3% (Player wins on 15/36 outcomes, loses on 21/36. (15/36) \* 2 = 30/36 = 0.833...).
- **Error Handling:** Basic error handling is implemented, sending structured error messages back to the client (see `constants.ErrCode*`) which are displayed on the UI. Production systems would require more nuanced error handling and monitoring.
- **Configuration:** Key values like the maximum bet amount (`MAX_BET_AMOUNT` env var) and HTTP server timeouts are loaded via `internal/config`. Other values like Redis lock expiry or specific bet types remain defined as constants but could be made configurable if needed.
- **Dependencies:**
  - Backend: Go standard library, `gorilla/websocket`, `pgx/v5`, `go-redis/v8`, `joho/godotenv`.
  - Frontend: SvelteKit, Svelte 5, TypeScript. Node.js runtime with PM2 in Docker.
- **Graceful Shutdown:** Implemented in `main.go` using `signal.NotifyContext` and `server.Shutdown` to handle `SIGINT`/`SIGTERM`.
