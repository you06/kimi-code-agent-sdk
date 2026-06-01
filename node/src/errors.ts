export class KimiCodeAgentError extends Error {
  readonly code: string;
  readonly retryable: boolean;
  readonly details: unknown;

  constructor(message: string, options: { code: string; retryable?: boolean; details?: unknown }) {
    super(message);
    this.name = 'KimiCodeAgentError';
    this.code = options.code;
    this.retryable = options.retryable ?? false;
    this.details = options.details;
  }
}

export class TransportError extends KimiCodeAgentError {
  constructor(message: string, details?: unknown) {
    super(message, { code: 'TRANSPORT_ERROR', retryable: true, details });
    this.name = 'TransportError';
  }
}

export class ProtocolError extends KimiCodeAgentError {
  constructor(message: string, options: { code: string; retryable?: boolean; details?: unknown }) {
    super(message, options);
    this.name = 'ProtocolError';
  }
}
