const SHA_REGEX = /(blobs|manifests)\/sha256:[a-f0-9]+$/;

export interface CachedResponse {
  responseText: string | null;
  dockerContentdigest: string | null;
}

/**
 * Extract SHA256 hash from URL if it's a cacheable request
 */
const getSha256 = (method: string, url: string): string | undefined => {
  if (method !== 'GET') {
    return;
  }
  const parts = SHA_REGEX.exec(url);
  if (!parts || !parts[0]) {
    return;
  }
  return parts[0];
};

/**
 * Get cached response from sessionStorage
 */
export const getFromCache = (method: string, url: string): CachedResponse | undefined => {
  const sha256 = getSha256(method, url);
  if (!sha256) {
    return;
  }
  try {
    return {
      responseText: sessionStorage.getItem(`${sha256}/responseText`),
      dockerContentdigest: sessionStorage.getItem(`${sha256}/dockerContentdigest`),
    };
  } catch {
    return undefined;
  }
};

/**
 * Store response in sessionStorage cache
 */
export const setCache = (
  method: string,
  url: string,
  { responseText, dockerContentdigest }: { responseText: string; dockerContentdigest: string | null },
): void => {
  const sha256 = getSha256(method, url);
  if (!sha256) {
    return;
  }
  try {
    sessionStorage.setItem(`${sha256}/responseText`, responseText);
    if (dockerContentdigest) {
      sessionStorage.setItem(`${sha256}/dockerContentdigest`, dockerContentdigest);
    }
  } catch {
    // Ignore storage errors (quota exceeded, etc.)
  }
};
