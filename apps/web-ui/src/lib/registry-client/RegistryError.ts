/**
 * Registry 错误
 */
export class RegistryError extends Error {
  public code?: string;
  public status?: number;
  public details?: unknown;

  constructor(message: string, code?: string, status?: number, details?: unknown) {
    super(message);
    this.name = 'RegistryError';
    this.code = code;
    this.status = status;
    this.details = details;
  }
}
