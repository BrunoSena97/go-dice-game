# Go_Dice-Game

## Description

This project contains the backend implementation for a simple dice game, developed as part of a technical assessment. The backend is written in Go and uses WebSockets for real-time client communication. It manages player wallets using PostgreSQL and utilizes Redis for intermediate play state management (concurrency control). A placeholder directory for a SvelteKit frontend is included.

The core game logic involves betting on whether the sum of two dice will be less than 7 ("lt7") or greater than 7 ("gt7"). A roll of exactly 7 results in a loss for the player. _(Note: This rule differs from the "even/odd" example initially described in the assessment PDF)._

## Project Structure

```
dice_game/
├── dice_game_backend/   # Go backend source code
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
│   ├── go.mod
│   └── go.sum
├── dice_game_frontend/  # Placeholder for SvelteKit frontend
├── db_init/             # SQL scripts for database initialization
│   └── 01-init.sql
├── .env.example         # Example environment variables
├── .gitignore           # Git ignore rules for Go & SvelteKit
├── docker-compose.yml   # Docker Compose configuration
└── README.md            # This file
```

_(Note: Structure reflects inclusion of `constants` package)_

## Requirements

- Docker
- Docker Compose
- Go (version 1.20+ recommended, for local backend dev)
- Node.js / npm (if developing the SvelteKit frontend)
- Git

## Setup

1.  **Clone the repository:**
    ```bash
    git clone <your-repository-url>
    cd dice-game # Or your repository root name
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
      - _(Optional)_ `MAX_BET_AMOUNT` if you want to override the default max bet (250).
    - **Important:** The `.env` file is ignored by Git (`.gitignore`) and should **not** be committed.

## Running the Project

The easiest way to run the application and its dependencies (PostgreSQL, Redis) is using Docker Compose.

1.  **Build and Start Containers:**
    From the project root directory (`dice_game/`), run:

    ```bash
    docker-compose up --build -d
    ```

    - `--build`: Ensures the Go backend image is built with the latest code.
    - `-d`: Runs the containers in the background.
    - This starts the `backend`, `db`, and `redis` services.

2.  **Accessing the Service:**

    - The backend WebSocket server will be running and accessible at `ws://localhost:8080/ws` (or the port specified by `BACKEND_PORT_HOST` in your `.env` file).
    - PostgreSQL is accessible on `localhost:5433` (or `DB_PORT_HOST`) for debugging if needed.
    - Redis is accessible on `localhost:6380` (or the host port you mapped) for debugging if needed.

3.  **Stopping the Services:**

    ```bash
    docker-compose down
    ```

    To also remove the persistent database volume (deletes all data):

    ```bash
    docker-compose down -v
    ```

4.  **Local Development (Backend Only):**
    If you want to run the Go backend directly for faster iteration:
    - Ensure DB and Redis are running: `docker-compose up -d db redis`
    - Navigate to the backend directory: `cd dice_game_backend`
    - Ensure Go dependencies: `go mod tidy`
    - Run the server with the `-dev` flag:
      ```bash
      go run ./cmd/server/main.go -dev
      ```
    - _(Note: Frontend development would typically involve running its dev server separately, ex: `cd ../dice_game_frontend && npm run dev`)_

## API / WebSocket Protocol

Communication happens over a single WebSocket endpoint: `/ws`. Messages are JSON strings.

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

The project should be tested locally using the Docker Compose setup.

1.  **WebSocket Client:** Use a tool capable of sending WebSocket messages, such as:
    - `wscat` (Node.js tool)
    - Postman (using the WebSocket Request feature)
    - Browser-based WebSocket clients.
      Connect to `ws://localhost:8080/ws` (adjust port if needed).
2.  **Postman Collection:** A Postman collection demonstrating example requests should accompany this project. _(!TODO: Needs to be created)_. The collection should show how to connect and send example `play`, `get_balance`, and `end_play` messages.
3.  **Example Flow:**
    - Connect to the WebSocket endpoint.
    - Optionally send `get_balance` (wallet created with 500 balance on first action if not existing).
    - Send a `play` message (ex: `{"type":"play", "payload":{"clientId":"user123", "betAmount": 50, "betType": "lt7"}}`).
    - Observe `play_result` and `balance_update` responses.
    - Send subsequent `play` messages (should work immediately after previous completes).
    - Send an `end_play` message (`{"type":"end_play", "payload":{"clientId":"user123"}}`) to get final balance and have the server disconnect.
    - Test error conditions (insufficient funds, invalid bets, concurrent plays - expect "ACTIVE_PLAY_EXISTS" error).
4.  **Debugging:**
    - Backend Logs: `docker-compose logs -f backend`
    - Redis: `docker-compose exec redis redis-cli` (use `KEYS *`, `GET keyname`, `TTL keyname`)
    - Database: `docker-compose exec db psql -U your_user -d your_db` (use `SELECT * FROM wallets;`) (Replace `your_user`/`your_db` with values from `.env`)

## Assumptions & Deviations & Design Choices

- **ClientID Handling:** For simplicity in this assessment, the `ClientID` is taken directly from the message payloads sent by the client. The backend currently trusts this ID. **A production system would require a secure authentication mechanism** (ex: tokens via initial HTTP auth or ws message) to establish and validate the user's identity associated with a WebSocket connection.
- **`end_play` Workflow:** The implemented workflow **deviates** from the _example_ sequence shown in the assessment PDF. In the PDF example, `"play"` returns the result, and `"end_play"` credits the winnings. In _this implementation_, the `"play"` handler completes the entire round atomically: it debits the bet, determines the outcome (via `game.Service`), **credits any winnings immediately** using the `wallet.Service`, and then sends the results back. The `active_play` Redis key acts only as a short-lived lock (~15s expiry) to prevent _concurrent_ processing for the same client, and is deleted promptly after processing. The `"end_play"` message is now only used to retrieve the final balance and trigger a server-side disconnect; it does not credit winnings. This change was made to simplify the state management and create a more atomic play loop, while still preventing overlapping processing via the Redis lock.
- **Game Rules:** The game logic was implemented as "Sum of 2 Dice < 7 / > 7 / 7 loses" based on development discussions, differing from the "Even/Odd" example in the PDF.
- **RTP:** The payout for a win is 1:1 (meaning the player receives their stake back _plus_ an amount equal to their stake). With the current "<7 / >7 / 7 loses" rules on 2 dice, this results in an approximate Return To Player (RTP) of 83.3% (Player wins on 15/36 outcomes, loses on 21/36. (15/36) \* 2 = 30/36 = 0.833...).
- **Error Handling:** Basic error handling is implemented, sending structured error messages back to the client (see `constants.ErrCode*`). Production systems would require more nuanced error handling and monitoring.
- **Configuration:** Key values like the maximum bet amount (`MAX_BET_AMOUNT` env var) and HTTP server timeouts are loaded via `internal/config`. Other values like Redis lock expiry or specific bet types remain defined as constants but could be made configurable if needed.
- **Dependencies:** Uses Go standard library, `gorilla/websocket`, `pgx/v5`, `go-redis/v8`, `joho/godotenv`.
- **Graceful Shutdown:** Implemented in `main.go` using `signal.NotifyContext` and `server.Shutdown` to handle `SIGINT`/`SIGTERM`.

## TODO / Future Work

- **Implement robust Authentication and ClientID management:** Securely associate WebSocket connections with authenticated users.
- **Add Unit and Integration Tests:** Cover services, handlers, and key workflows.
- **Enhance Logging:** Implement structured logging (`slog`) for better monitoring and analysis.
- **Refine Configuration:** Consider moving more constants (ex: Redis timeouts, bet types) to configuration if deployment flexibility is required.
