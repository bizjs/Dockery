import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const LOCAL_STORAGE_KEY = 'registryServer';

export function stripHttps(url?: string): string {
  if (!url) {
    return '';
  }
  return url.replace(/^https?:\/\//, '');
}

export function isDigit(char: string): boolean {
  return char >= '0' && char <= '9';
}

export const ERROR_CAN_NOT_READ_CONTENT_DIGEST =
  'Access on registry response was blocked. Try adding the header ' +
  '`Access-Control-Expose-Headers: Docker-Content-Digest`' +
  ' to your proxy or registry: ' +
  'https://docs.docker.com/registry/configuration/#http';

export function getRegistryServers(i?: number): string | string[] {
  try {
    const res = JSON.parse(localStorage.getItem(LOCAL_STORAGE_KEY) || '[]');
    if (res instanceof Array) {
      return !isNaN(i as number) ? res[i as number] : res.map((url: string) => url.trim().replace(/\/*$/, ''));
    }
  } catch {
    // ignore
  }
  return !isNaN(i as number) ? '' : [];
}

export function setRegistryServers(registries: string | string[]): void {
  let registryArray: string[];
  if (typeof registries === 'string') {
    registryArray = registries.split(',');
  } else if (!Array.isArray(registries)) {
    throw new Error('setRegistries must be called with string or array parameter');
  } else {
    registryArray = registries;
  }
  registryArray = registryArray.map((registry) => registry.replace(/\/*$/, ''));
  localStorage.setItem(LOCAL_STORAGE_KEY, JSON.stringify(registryArray));
}

export function addRegistryServers(registry: string): string {
  const url = registry.trim().replace(/\/*$/, '');
  const registryServer = (getRegistryServers() as string[]).filter((e) => e !== url);
  setRegistryServers([url].concat(registryServer));
  return url;
}

export function removeRegistryServers(registry: string): void {
  const registryServers = (getRegistryServers() as string[]).filter((e) => e !== registry);
  setRegistryServers(registryServers);
}

export function encodeURI(url?: string): string | undefined {
  if (!url) {
    return;
  }
  return url.indexOf('&') < 0 ? window.encodeURIComponent(url) : btoa(url);
}

export function decodeURI(url?: string): string | undefined {
  if (!url) {
    return;
  }
  return url.startsWith('http') ? window.decodeURIComponent(url) : atob(url);
}

export function truthy(value: boolean | string): boolean {
  return value === true || value === 'true';
}

export function falsy(value: boolean | string): boolean {
  return value !== false && value !== 'false';
}
