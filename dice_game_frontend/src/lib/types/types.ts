// Client -> Server

export interface BaseWsMessage {
	type: string;
	payload: unknown;
}

export interface PlayPayload {
	clientId: string;
	betAmount: number;
	betType: 'lt7' | 'gt7';
}

export interface GetBalancePayload {
	clientId: string;
}

export interface EndPlayPayload {
	clientId: string;
}

// Server -> Client

export interface BaseServerMessage {
	type: string;
	payload: unknown;
}

export interface PlayResultPayload {
	clientId: string;
	die1: number;
	die2: number;
	outcome: 'win' | 'lose';
	betAmount: number;
	winnings: number;
}

export interface BalanceUpdatePayload {
	clientId: string;
	balance: number;
}

export interface PlayEndedPayload {
	clientId: string;
	finalBalance: number;
}

export interface ErrorPayload {
	code: string;
	message: string;
}

// Type guard to check server message type
export function isServerMessage<T>(
	msg: unknown,
	type: string
): msg is { type: string; payload: T } {
	return (
		typeof msg === 'object' &&
		msg !== null &&
		'type' in msg &&
		msg.type === type &&
		'payload' in msg
	);
}
