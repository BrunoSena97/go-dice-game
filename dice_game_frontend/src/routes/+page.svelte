<script lang="ts">
	import { browser } from '$app/environment';
	import type {
		PlayPayload,
		GetBalancePayload,
		EndPlayPayload,
		PlayResultPayload,
		BalanceUpdatePayload,
		PlayEndedPayload,
		ErrorPayload,
		BaseWsMessage,
		BaseServerMessage
	} from '$lib/types/types';
	import { isServerMessage } from '$lib/types/types';

	// Config
	const socketURL = 'ws://localhost:8080/ws';
	const chipValues = [1, 2, 10, 25, 50, 100];
	const diceRollAnimationTime = 1500; // ms

	// State
	type UIState = 'disconnected' | 'connecting' | 'connected' | 'error';
	let uiState = $state<UIState>('disconnected');
	let socket = $state<WebSocket | null>(null);
	let clientId = $state('');
	let balance = $state<number | null>(null);
	let currentBet = $state(0);
	let betChoice = $state<'lt7' | 'gt7' | null>(null);
	let isRolling = $state(false);
	let errorMsg = $state('');
	let lastResult = $state<PlayResultPayload | null>(null);
	let diceDisplay = $state({ d1: '?', d2: '?' });

	// Derived State
	const isConnected = $derived(uiState === 'connected');
	const canPlay = $derived(
		isConnected &&
			!isRolling &&
			currentBet > 0 &&
			!!betChoice &&
			(balance === null || balance >= currentBet)
	);
	const canSelectChips = $derived(isConnected && !isRolling);

	// Effects
	$effect(() => {
		if (browser && !clientId) {
			clientId = `client_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`;
			console.log('Generated Client ID:', clientId);
		}
	});

	$effect(() => {
		const currentSocket = socket;
		return () => {
			if (currentSocket && currentSocket.readyState < WebSocket.CLOSING) {
				console.log('Effect cleanup: Closing WebSocket connection...', currentSocket.url);
				try {
					currentSocket.close(1000, 'Component unmounting or state change');
				} catch (e) {
					console.warn('Error closing socket during cleanup:', e);
				}

				if (uiState === 'connected' || uiState === 'connecting') {
					uiState = 'disconnected';
					balance = null;
					lastResult = null;
					isRolling = false;
				}
			}
		};
	});

	$effect(() => {
		if (errorMsg) {
			const timer = setTimeout(() => {
				errorMsg = '';
				if (uiState === 'error') uiState = 'disconnected';
			}, 5000);
			return () => clearTimeout(timer);
		}
	});

	// Functions
	function startGame() {
		if (uiState !== 'disconnected' && uiState !== 'error') return;
		if (!clientId) {
			errorMsg = 'Cannot start: Client ID not generated.';
			uiState = 'error';
			return;
		}

		console.log('User initiated connection...');
		uiState = 'connecting';
		errorMsg = '';
		lastResult = null;
		diceDisplay = { d1: '?', d2: '?' };
		resetBet();

		connectWebSocket();
	}

	function connectWebSocket() {
		if (socket && socket.readyState < WebSocket.CLOSING) {
			console.log('WebSocket connection attempt already in progress or open.');
			return;
		}

		socket = new WebSocket(socketURL);

		socket.onopen = () => {
			console.log('WebSocket Connected');
			uiState = 'connected';
			errorMsg = '';
			sendMessage<GetBalancePayload>('get_balance', { clientId });
		};

		socket.onmessage = (event: MessageEvent) => {
			console.log('Message from server:', event.data);
			try {
				const message: BaseServerMessage = JSON.parse(event.data);
				handleIncomingMessage(message);
			} catch (e) {
				console.error('Failed to parse message or invalid message format:', e);
				errorMsg = 'Received invalid message from server.';
			}
		};

		socket.onerror = (event: Event) => {
			console.error('WebSocket Error:', event);
			errorMsg = 'Connection failed. Check console or backend logs.';
			uiState = 'error';
			socket = null;
			isRolling = false;
		};

		socket.onclose = (event: CloseEvent) => {
			console.log('WebSocket Disconnected:', event.code, event.reason);
			if (!event.wasClean && uiState !== 'disconnected') {
				errorMsg = 'Connection closed unexpectedly.';
			}

			uiState = 'disconnected';
			socket = null;
			balance = null;
			lastResult = null;
			isRolling = false;
			currentBet = 0;
			betChoice = null;
		};
	}

	function sendMessage<T>(type: string, payload: T) {
		if (socket && socket.readyState === WebSocket.OPEN && uiState === 'connected') {
			const message: BaseWsMessage = { type, payload };
			console.log('Sending message:', message);
			socket.send(JSON.stringify(message));
		} else {
			console.error('WebSocket not connected or not open. Cannot send message.');
			errorMsg = 'Not connected to server.';
			if (uiState !== 'error') uiState = 'disconnected';
		}
	}

	function handleIncomingMessage(message: BaseServerMessage) {
		if (uiState !== 'error') errorMsg = '';

		if (isServerMessage<BalanceUpdatePayload>(message, 'balance_update')) {
			if (uiState === 'connected') balance = message.payload.balance;
		} else if (isServerMessage<PlayResultPayload>(message, 'play_result')) {
			if (uiState === 'connected') {
				console.log('Received play_result');
				lastResult = message.payload;
				diceDisplay.d1 = String(lastResult.die1);
				diceDisplay.d2 = String(lastResult.die2);
				isRolling = false;
				currentBet = 0;
			}
		} else if (isServerMessage<PlayEndedPayload>(message, 'play_ended')) {
			if (uiState === 'connected') {
				balance = message.payload.finalBalance;
				console.log('Play ended acknowledged by server. Connection will close.');
				errorMsg = `Game ended. Final Balance: ${balance}. Server closing connection.`;
			}
		} else if (isServerMessage<ErrorPayload>(message, 'error')) {
			console.error('Server Error:', message.payload);
			errorMsg = `Error: ${message.payload.message} (${message.payload.code})`;
			isRolling = false;
			if (
				message.payload.code === 'INSUFFICIENT_FUNDS' ||
				message.payload.code === 'INVALID_BET' ||
				message.payload.code === 'BET_TOO_HIGH'
			) {
				currentBet = 0;
			}
		} else {
			console.warn('Received unknown message type:', message.type);
		}
	}

	function addBet(amount: number) {
		if (!canSelectChips) return;
		currentBet += amount;
		lastResult = null;
		diceDisplay = { d1: '?', d2: '?' };
	}

	function setBetChoice(choice: 'lt7' | 'gt7') {
		if (!canSelectChips) return;
		betChoice = choice;
		lastResult = null;
		diceDisplay = { d1: '?', d2: '?' };
	}

	function resetBet() {
		currentBet = 0;
		betChoice = null;
		lastResult = null;
		diceDisplay = { d1: '?', d2: '?' };
		errorMsg = '';
	}

	function handlePlay() {
		if (!canPlay || !betChoice) {
			console.warn('Play cancelled:', { canPlay, betChoice });
			return;
		}

		lastResult = null;
		errorMsg = '';
		isRolling = true;
		diceDisplay = { d1: '?', d2: '?' };

		const intervalId = setInterval(() => {
			diceDisplay.d1 = String(Math.floor(Math.random() * 6) + 1);
			diceDisplay.d2 = String(Math.floor(Math.random() * 6) + 1);
		}, 100);

		const safetyTimeout = setTimeout(() => {
			if (isRolling) {
				console.warn('Safety timeout stopped dice rolling animation.');
				clearInterval(intervalId);
				isRolling = false;
				if (!lastResult && uiState === 'connected') errorMsg = 'No result received from server.';
			}
		}, diceRollAnimationTime + 2000);

		if (!socket) {
			console.error('Cannot modify onmessage handler: socket is null.');
			clearInterval(intervalId);
			clearTimeout(safetyTimeout);
			isRolling = false;
			errorMsg = 'Internal error: connection lost before sending play.';
			uiState = 'error';
			return;
		}
		const originalOnMessage = socket.onmessage;

		socket.onmessage = (event: MessageEvent) => {
			console.log('Play response received, stopping animation.');
			clearInterval(intervalId);
			clearTimeout(safetyTimeout);

			if (socket) {
				socket.onmessage = originalOnMessage;
			}

			if (originalOnMessage) {
				if (socket) {
					originalOnMessage.call(socket, event);
				} else {
					console.error('Socket became null before original handler could be called');
				}
			} else {
				try {
					const message: BaseServerMessage = JSON.parse(event.data);
					handleIncomingMessage(message);
				} catch (e) {
					console.error('Error parsing message after animation:', e);
					errorMsg = 'Received invalid response from server.';
				}
			}
		};

		const payload: PlayPayload = {
			clientId,
			betAmount: currentBet,
			betType: betChoice
		};
		sendMessage<PlayPayload>('play', payload);
	}

	function handleEndPlay() {
		if (!isConnected) return;
		console.log('Requesting to end play...');
		sendMessage<EndPlayPayload>('end_play', { clientId });
	}
</script>

<main class="container">
	<h1>Dice Game</h1>

	{#if uiState === 'disconnected' || uiState === 'error'}
		<section class="start-section">
			<h2>Welcome!</h2>
			<p>Click below to connect and start playing.</p>
			<button class="start-button" onclick={startGame}> Connect & Play Game </button>
			{#if errorMsg}
				<p class="error-message">{errorMsg}</p>
			{/if}
		</section>
	{/if}

	{#if uiState === 'connecting'}
		<section class="status">
			<p class="loading">Connecting to server...</p>
		</section>
	{/if}

	{#if uiState === 'connected'}
		<section class="status">
			<p>Status: <span class="connected">Connected</span></p>
			<p>Client ID: <span class="client-id">{clientId}</span></p>
			<p>
				Balance: <span class="balance">{balance === null ? 'Loading...' : `${balance} PTS`}</span>
			</p>
		</section>

		<section class="rules">
			<h2>Rules</h2>
			<ul>
				<li>Place a bet by selecting chips and choosing 'Under 7' or 'Over 7'.</li>
				<li>Click 'Play' to roll two dice.</li>
				<li>If the sum is LESS than 7 and you bet 'Under 7', you win 1:1.</li>
				<li>If the sum is GREATER than 7 and you bet 'Over 7', you win 1:1.</li>
				<li>If the sum is EXACTLY 7, you lose.</li>
				<li>Click 'End Play' to disconnect.</li>
			</ul>
		</section>

		<section class="betting">
			<h2>Place Your Bet</h2>
			<div class="chips">
				{#each chipValues as chip}
					<button class="chip chip-{chip}" onclick={() => addBet(chip)} disabled={!canSelectChips}>
						{chip}
					</button>
				{/each}
			</div>
			{#if currentBet > 0}
				<p class="current-bet">Current Bet: <span>{currentBet}</span> PTS</p>
				<div class="bet-choice">
					<button
						class:selected={betChoice === 'lt7'}
						disabled={!canSelectChips}
						onclick={() => setBetChoice('lt7')}
					>
						Under 7 (lt7)
					</button>
					<button
						class:selected={betChoice === 'gt7'}
						disabled={!canSelectChips}
						onclick={() => setBetChoice('gt7')}
					>
						Over 7 (gt7)
					</button>
				</div>
				<button class="reset-button" onclick={resetBet} disabled={!canSelectChips}>Reset Bet</button
				>
			{/if}
		</section>

		<section class="actions">
			<button class="play-button" onclick={handlePlay} disabled={!canPlay}>
				{isRolling ? 'Rolling...' : 'Play'}
			</button>
			<button class="end-button" onclick={handleEndPlay} disabled={isRolling}> End Play </button>
		</section>

		<section class="game-area">
			<h2>Dice Roll</h2>
			<div class="dice-container">
				<div class="dice">{isRolling ? diceDisplay.d1 : lastResult ? lastResult.die1 : '?'}</div>
				<div class="dice">{isRolling ? diceDisplay.d2 : lastResult ? lastResult.die2 : '?'}</div>
			</div>
			{#if lastResult}
				<div class="results">
					<p>Sum: <span>{lastResult.die1 + lastResult.die2}</span></p>
					<p>Your Bet: <span>{lastResult.betAmount}</span></p>
					<p class:win={lastResult.outcome === 'win'} class:lose={lastResult.outcome === 'lose'}>
						Outcome: <span>{lastResult.outcome.toUpperCase()}</span>
					</p>
					{#if lastResult.outcome === 'win'}
						<p class="winnings">Net Winnings: <span>{lastResult.winnings}</span> PTS</p>
					{/if}
				</div>
			{/if}
		</section>

		{#if errorMsg && uiState === 'connected'}
			<p class="error-message">{errorMsg}</p>
		{/if}
	{/if}
</main>

<style>
	.container {
		font-family: sans-serif;
		max-width: 800px;
		margin: 2em auto;
		padding: 1em;
		border: 1px solid #ccc;
		border-radius: 8px;
		background-color: #f9f9f9;
	}

	h1,
	h2 {
		text-align: center;
		color: #333;
	}

	section {
		margin-bottom: 2em;
		padding: 1em;
		border-bottom: 1px solid #eee;
		padding-top: 0;
		padding-bottom: 2;
	}
	section:last-child {
		border-bottom: none;
	}
	.start-section {
		text-align: center;
	}
	.start-button {
		padding: 12px 25px;
		font-size: 1.2em;
		cursor: pointer;
		background-color: #007bff;
		color: white;
		border: none;
		border-radius: 5px;
		margin-top: 1em;
	}
	.start-button:hover {
		background-color: #0056b3;
	}

	.status {
		display: flex;
		flex-direction: row;
		flex-wrap: wrap;
		justify-content: space-around;
		align-items: center;
		gap: 1em;
		margin-bottom: 1.5em;
		padding: 0.5em 1em;
		border-bottom: 1px solid #eee;
	}

	.status p,
	.betting p,
	.results p {
		margin: 0.5em 0;
	}
	.status .client-id {
		font-family: monospace;
		font-size: 0.9em;
		color: #555;
	}
	.status .balance {
		font-weight: bold;
		color: green;
	}

	.status .connected {
		color: green;
		font-weight: bold;
	}

	.rules ul {
		list-style: disc;
		margin-left: 20px;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		margin-bottom: 1em;
		justify-content: center;
	}

	.chip {
		border-radius: 50%;
		width: 50px;
		height: 50px;
		border: 3px solid #555;
		font-weight: bold;
		font-size: 1.1em;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		box-shadow: 2px 2px 5px rgba(0, 0, 0, 0.2);
		transition: transform 0.1s ease;
	}
	.chip:disabled {
		opacity: 0.5;
		cursor: not-allowed;
		box-shadow: none;
		transform: none;
	}
	.chip:not(:disabled):hover {
		transform: scale(1.1);
	}
	.chip-1 {
		background-color: white;
		color: #333;
		border-color: #aaa;
	}
	.chip-2 {
		background-color: red;
		color: white;
		border-color: darkred;
	}
	.chip-10 {
		background-color: blue;
		color: white;
		border-color: darkblue;
	}
	.chip-25 {
		background-color: green;
		color: white;
		border-color: darkgreen;
	}
	.chip-50 {
		background-color: orange;
		color: white;
		border-color: darkorange;
	}
	.chip-100 {
		background-color: black;
		color: white;
		border-color: #444;
	}

	.current-bet span {
		font-weight: bold;
		font-size: 1.2em;
	}
	.current-bet {
		text-align: center;
	}

	.bet-choice {
		margin: 1em 0;
		display: flex;
		gap: 10px;
		justify-content: center;
	}
	.bet-choice button {
		padding: 8px 15px;
		font-size: 1em;
		cursor: pointer;
		border: 2px solid transparent;
		border-radius: 5px;
		background-color: #eee;
	}
	.bet-choice button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.bet-choice button.selected {
		border-color: #007bff;
		background-color: #cce5ff;
		font-weight: bold;
	}

	.reset-button {
		display: block;
		margin: 0.5em auto;
		padding: 5px 10px;
		background-color: #ffc107;
		border: none;
		border-radius: 4px;
		cursor: pointer;
	}
	.reset-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.actions {
		display: flex;
		justify-content: center;
		gap: 20px;
	}
	.play-button,
	.end-button {
		padding: 10px 25px;
		font-size: 1.2em;
		cursor: pointer;
		border-radius: 5px;
		border: none;
		color: white;
	}
	.play-button {
		background-color: #28a745;
	}
	.end-button {
		background-color: #dc3545;
	}
	.play-button:disabled,
	.end-button:disabled {
		background-color: #ccc;
		cursor: not-allowed;
	}

	.game-area {
		text-align: center;
	}
	.dice-container {
		display: flex;
		justify-content: center;
		gap: 20px;
		margin-bottom: 1em;
		min-height: 40px;
	}
	.dice {
		width: 50px;
		height: 50px;
		border: 2px solid black;
		border-radius: 5px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 2em;
		font-weight: bold;
		background-color: white;
	}

	.results {
		margin-top: 1em;
		padding: 0.5em;
		background-color: #e9ecef;
		border-radius: 5px;
		display: inline-block;
	}
	.results span {
		font-weight: bold;
	}
	.results .win {
		color: green;
	}
	.results .lose {
		color: red;
	}
	.results .winnings span {
		color: darkgreen;
	}

	.loading {
		text-align: center;
		font-style: italic;
		color: #666;
	}
	.error-message {
		color: red;
		font-weight: bold;
		text-align: center;
		margin-top: 1em;
		padding: 0.5em;
		background-color: #ffebeb;
		border: 1px solid red;
		border-radius: 4px;
	}
</style>
