const DEFAULT_BASE_URL = 'https://kk.642636.xyz';
const MAX_ATTEMPTS = 4;
const BASE_RETRY_DELAY_MS = 250;
const MAX_RETRY_DELAY_MS = 30_000;
const RETRYABLE_STATUSES = new Set([429, 502, 503]);
const COOKIE_KEY = 'helan_client';

function defaultSleep(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function retryAfterMs(response, fallbackMs, maximumMs) {
  const value = response.headers.get('Retry-After');
  if (!value) {
    return fallbackMs;
  }

  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) {
    return Math.min(seconds * 1000, maximumMs);
  }

  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return fallbackMs;
  }

  return Math.min(Math.max(0, timestamp - Date.now()), maximumMs);
}

function responseCookies(headers) {
  if (typeof headers.getSetCookie === 'function') {
    return headers.getSetCookie();
  }

  const value = headers.get('Set-Cookie');
  return value ? [value] : [];
}

function extractSessionCookie(headers) {
  for (const cookie of responseCookies(headers)) {
    const match = cookie.match(/(?:^|,\s*)helan_client=([^;\s,]+)/);
    if (match) {
      return match[1];
    }
  }

  return null;
}

async function readErrorBody(response) {
  const text = await response.text();
  if (!text) {
    return '';
  }

  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function errorMessage(body, status) {
  if (body && typeof body === 'object' && typeof body.error === 'string') {
    return body.error;
  }
  if (typeof body === 'string' && body.trim()) {
    return body.trim();
  }
  return `Upstream request failed with status ${status}`;
}

export class UpstreamError extends Error {
  constructor(message, {
    status,
    body,
    retryable = false,
    uncertain = false,
    cause,
  } = {}) {
    super(message, { cause });
    this.name = 'UpstreamError';
    this.status = status;
    this.body = body;
    this.retryable = retryable;
    this.uncertain = uncertain;
  }
}

export function createUpstreamClient({
  baseUrl = DEFAULT_BASE_URL,
  fetchImpl = globalThis.fetch,
  cookieStore,
  sleep = defaultSleep,
} = {}) {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '');

  function requestHeaders(contentType) {
    const headers = { Accept: contentType };
    if (contentType === 'application/json') {
      headers['Content-Type'] = 'application/json';
    }

    const cookie = cookieStore?.getSetting(COOKIE_KEY);
    if (cookie) {
      headers.Cookie = `${COOKIE_KEY}=${cookie}`;
    }

    return headers;
  }

  async function request(path, options, {
    retryNetworkErrors = false,
    uncertainOnNetworkError = false,
  } = {}) {
    for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt += 1) {
      let response;

      try {
        response = await fetchImpl(`${normalizedBaseUrl}${path}`, options());
      } catch (cause) {
        if (retryNetworkErrors && attempt < MAX_ATTEMPTS - 1) {
          const delayMs = Math.min(
            BASE_RETRY_DELAY_MS * (2 ** attempt),
            MAX_RETRY_DELAY_MS,
          );
          await sleep(delayMs);
          continue;
        }

        throw new UpstreamError('Upstream network request failed', {
          retryable: retryNetworkErrors,
          uncertain: uncertainOnNetworkError,
          cause,
        });
      }

      const cookie = extractSessionCookie(response.headers);
      if (cookie) {
        cookieStore?.setSetting(COOKIE_KEY, cookie);
      }

      if (!RETRYABLE_STATUSES.has(response.status) || attempt === MAX_ATTEMPTS - 1) {
        return response;
      }

      const fallbackMs = Math.min(
        BASE_RETRY_DELAY_MS * (2 ** attempt),
        MAX_RETRY_DELAY_MS,
      );
      await sleep(retryAfterMs(response, fallbackMs, MAX_RETRY_DELAY_MS));
    }

    throw new Error('Unreachable retry state');
  }

  async function jsonRequest(path, body, acceptedStatuses, requestOptions) {
    const response = await request(path, () => ({
      method: 'POST',
      headers: requestHeaders('application/json'),
      body: JSON.stringify(body),
    }), requestOptions);
    const parsedBody = await readErrorBody(response);

    if (!acceptedStatuses.has(response.status)) {
      throw new UpstreamError(errorMessage(parsedBody, response.status), {
        status: response.status,
        body: parsedBody,
        retryable: RETRYABLE_STATUSES.has(response.status),
      });
    }
    if (!parsedBody || typeof parsedBody !== 'object') {
      throw new UpstreamError('Upstream returned an invalid JSON response', {
        status: response.status,
        body: parsedBody,
      });
    }

    return parsedBody;
  }

  return Object.freeze({
    submitExtractions(rows) {
      return jsonRequest('/api/extractions/batch', {
        accessTokens: rows.map((row) => row.accessToken),
        cdks: rows.map((row) => row.extractionCdk),
      }, new Set([202]), { uncertainOnNetworkError: true });
    },

    async getExtraction(taskId) {
      const response = await request(
        `/api/extractions/${encodeURIComponent(taskId)}`,
        () => ({
          method: 'GET',
          headers: requestHeaders('text/plain'),
        }),
        { retryNetworkErrors: true },
      );
      const body = (await response.text()).trim();

      if (response.status === 202) {
        return {
          status: 'processing',
          retryAfterMs: retryAfterMs(response, 2000, 10_000),
        };
      }
      if (response.status !== 200) {
        throw new UpstreamError(errorMessage(body, response.status), {
          status: response.status,
          body,
          retryable: RETRYABLE_STATUSES.has(response.status),
        });
      }

      try {
        const paymentUrl = new URL(body);
        if (paymentUrl.protocol === 'https:') {
          return { status: 'succeeded', paymentUrl: body };
        }
      } catch {
        // The upstream uses plain text for both successful links and failures.
      }

      return { status: 'failed', error: body || '提链失败' };
    },

    submitPayments(paymentCdk, urls) {
      return jsonRequest('/api/payments/submissions', {
        paymentCdk,
        paymentUrls: urls,
      }, new Set([202, 207]), { uncertainOnNetworkError: true });
    },

    getPaymentStatuses(ids) {
      return jsonRequest(
        '/api/payments/submissions/status',
        { ids },
        new Set([200]),
        { retryNetworkErrors: true },
      );
    },
  });
}
