export class DockerRegistryUIError extends Error {
  isError: boolean;
  code?: string | number;

  constructor(msg: string, code?: string | number) {
    super(msg);
    this.name = 'DockerRegistryUIError';
    this.isError = true;
    this.code = code;

    // Maintains proper stack trace for where our error was thrown (only available on V8)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (typeof (Error as any).captureStackTrace === 'function') {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (Error as any).captureStackTrace(this, DockerRegistryUIError);
    }
  }
}
