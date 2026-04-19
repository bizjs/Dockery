/**
 * HTTP client for Docker Registry API
 * Modern fetch-based implementation with authentication support
 */

import { getFromCache, setCache } from './cache-request'

export interface AuthToken {
  realm: string
  service: string
  scope: string
}

export interface BearerToken {
  token?: string
  access_token?: string
}

export interface HttpOptions {
  onAuthentication?: (auth: AuthToken | null, callback: (bearer: BearerToken | null) => void) => void
  withCredentials?: boolean
}

export interface HttpResponse<T = unknown> {
  data: T
  status: number
  headers: Headers
  contentDigest?: string
}

export interface RequestOptions {
  headers?: Record<string, string>
  getContentDigest?: boolean
}

export interface HttpError {
  code: string
  url: string
  message?: string
}

const AUTHENTICATE_HEADER_REGEX = /Bearer realm="(?<realm>[^"]+)",service="(?<service>[^"]+)",scope="(?<scope>[^"]+)"/

/**
 * Parse WWW-Authenticate header
 */
const parseAuthenticateHeader = (header: string): AuthToken | null => {
  const exec = AUTHENTICATE_HEADER_REGEX.exec(header)
  if (!exec?.groups) {
    return null
  }
  const groups = exec.groups
  if ('realm' in groups && 'service' in groups && 'scope' in groups) {
    return {
      realm: groups.realm,
      service: groups.service,
      scope: groups.scope,
    }
  }
  return null
}

/**
 * Get error message based on request details
 */
const getErrorMessage = (url: string, withCredentials: boolean, headers: Headers): HttpError | string => {
  if (url.match('^http://') && window.location.protocol === 'https:') {
    return { code: 'MIXED_CONTENT', url }
  } else if (!url || !url.match('^http')) {
    return { code: 'INCORRECT_URL', url }
  } else if (withCredentials && !headers.has('Access-Control-Allow-Credentials')) {
    return (
      "The `Access-Control-Allow-Credentials` header in the response is missing and must be set to `true` when the request's credentials mode is on. Origin `" +
      new URL(url).origin +
      '` is therefore not allowed access.'
    )
  }
  return (
    'An error occured: Check your connection and your registry must have `Access-Control-Allow-Origin` header set to `' +
    window.location.origin +
    '`'
  )
}

/**
 * Calculate SHA256 digest of response text
 */
const calculateDigest = async (text: string): Promise<string> => {
  if (window.crypto && window.TextEncoder) {
    const buffer = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
    return (
      'sha256:' +
      Array.from(new Uint8Array(buffer))
        .map((byte) => byte.toString(16).padStart(2, '0'))
        .join('')
    )
  }
  throw new Error('Crypto API not available')
}

/**
 * Modern HTTP client class using fetch API
 */
export class Http {
  private url: string = ''
  private method: string = 'GET'
  private headers: Record<string, string> = {}
  private onAuthentication?: (auth: AuthToken | null, callback: (bearer: BearerToken | null) => void) => void
  private withCredentials: boolean = false

  constructor(opts?: HttpOptions) {
    this.onAuthentication = opts?.onAuthentication
    this.withCredentials = opts?.withCredentials || false
  }

  /**
   * Set request header
   */
  setRequestHeader(header: string, value: string): void {
    this.headers[header] = value
  }

  /**
   * Open/configure the request
   */
  open(method: string, url: string): void {
    this.method = method
    this.url = url
  }

  /**
   * Send the request
   */
  async send(): Promise<Response> {
    // Check cache first
    const cache = getFromCache(this.method, this.url)
    if (cache?.responseText) {
      // Return a mock Response object for cached data
      return new Response(cache.responseText, {
        status: 200,
        headers: {
          'Docker-Content-Digest': cache.dockerContentdigest || '',
        },
      })
    }

    // Make the actual request
    const response = await this.makeRequest(this.url, this.headers, this.withCredentials)
    
    // Handle authentication
    if (response.status === 401 && this.onAuthentication) {
      const wwwAuth = response.headers.get('www-authenticate')
      const tokenAuth = wwwAuth ? parseAuthenticateHeader(wwwAuth) : null
      
      if (!this.withCredentials || tokenAuth) {
        return new Promise((resolve, reject) => {
          this.onAuthentication!(tokenAuth, async (bearer) => {
            try {
              const newHeaders = { ...this.headers }
              if (bearer?.token) {
                newHeaders['Authorization'] = `Bearer ${bearer.token}`
              } else if (bearer?.access_token) {
                newHeaders['Authorization'] = `Bearer ${bearer.access_token}`
              }
              
              const authResponse = await this.makeRequest(this.url, newHeaders, !bearer)
              resolve(authResponse)
            } catch (error) {
              reject(error)
            }
          })
        })
      }
    }

    // Cache successful responses
    if (response.status === 200) {
      const text = await response.clone().text()
      const dockerContentDigest = response.headers.get('Docker-Content-Digest')
      setCache(this.method, this.url, {
        responseText: text,
        dockerContentdigest: dockerContentDigest,
      })
    }

    return response
  }

  /**
   * Make the actual fetch request
   */
  private async makeRequest(url: string, headers: Record<string, string>, withCredentials: boolean): Promise<Response> {
    const fetchOptions: RequestInit = {
      method: this.method,
      headers,
    }

    if (withCredentials) {
      fetchOptions.credentials = 'include'
    }

    return fetch(url, fetchOptions)
  }

  /**
   * Get Docker Content Digest from response
   */
  async getContentDigest(response: Response): Promise<string | null> {
    // Check if header is available
    const digest = response.headers.get('Docker-Content-Digest')
    if (digest) {
      return digest
    }

    // Check cache
    const cache = getFromCache(this.method, this.url)
    if (cache?.dockerContentdigest) {
      return cache.dockerContentdigest
    }

    // Calculate from response text
    try {
      const text = await response.clone().text()
      return await calculateDigest(text)
    } catch {
      // If crypto is not available (old browsers), return null
      return null
    }
  }

  /**
   * Get error message for failed request
   */
  getErrorMessage(response: Response): HttpError | string {
    return getErrorMessage(this.url, this.withCredentials, response.headers)
  }
}

/**
 * Simple fetch wrapper for basic requests
 */
export const httpGet = async (url: string, headers: Record<string, string> = {}): Promise<Response> => {
  const http = new Http()
  http.open('GET', url)
  Object.entries(headers).forEach(([key, value]) => {
    http.setRequestHeader(key, value)
  })
  return http.send()
}

/**
 * Fetch JSON data
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const httpGetJson = async <T = any>(url: string, headers: Record<string, string> = {}): Promise<T> => {
  const response = await httpGet(url, headers)
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`)
  }
  return response.json()
}

/**
 * Enhanced HTTP client with content digest support
 */
export const http = {
  async get<T = unknown>(url: string, options: RequestOptions = {}): Promise<HttpResponse<T>> {
    const httpClient = new Http()
    httpClient.open('GET', url)
    
    if (options.headers) {
      Object.entries(options.headers).forEach(([key, value]) => {
        httpClient.setRequestHeader(key, value)
      })
    }
    
    const response = await httpClient.send()
    
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`)
    }
    
    const data = await response.json() as T
    const result: HttpResponse<T> = {
      data,
      status: response.status,
      headers: response.headers,
    }
    
    if (options.getContentDigest) {
      const digest = await httpClient.getContentDigest(response)
      result.contentDigest = digest || undefined
    }
    
    return result
  }
}
