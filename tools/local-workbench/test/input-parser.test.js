import test from 'node:test';
import assert from 'node:assert/strict';

import { parsePastedRows, validateRows } from '../src/input-parser.js';

test('parsePastedRows parses and trims tab-separated rows', () => {
  const rows = parsePastedRows(
    ' token-1\t extraction-1 \t payment-1\n' +
      'token-2\textraction-2\tpayment-2 ',
  );

  assert.deepEqual(rows, [
    {
      accessToken: 'token-1',
      extractionCdk: 'extraction-1',
      paymentCdk: 'payment-1',
    },
    {
      accessToken: 'token-2',
      extractionCdk: 'extraction-2',
      paymentCdk: 'payment-2',
    },
  ]);
});

test('parsePastedRows parses comma-separated rows', () => {
  const rows = parsePastedRows('token-1, extraction-1, payment-1');

  assert.deepEqual(rows, [
    {
      accessToken: 'token-1',
      extractionCdk: 'extraction-1',
      paymentCdk: 'payment-1',
    },
  ]);
});

test('parsePastedRows ignores blank lines', () => {
  const rows = parsePastedRows(
    '\n token-1\textraction-1\tpayment-1\n   \n\t\t\n',
  );

  assert.deepEqual(rows, [
    {
      accessToken: 'token-1',
      extractionCdk: 'extraction-1',
      paymentCdk: 'payment-1',
    },
  ]);
});

test('validateRows reports each incomplete row using one-based row numbers', () => {
  const rows = parsePastedRows(
    'token-1\textraction-1\tpayment-1\n' +
      'token-2\textraction-2\n' +
      ',extraction-3,payment-3',
  );

  const result = validateRows(rows);

  assert.equal(result.valid, false);
  assert.deepEqual(
    result.errors.map(({ row }) => row),
    [2, 3],
  );
  assert.match(result.errors[0].message, /payment CDK/i);
  assert.match(result.errors[1].message, /access token/i);
});

test('validateRows accepts complete rows', () => {
  const rows = parsePastedRows('token-1\textraction-1\tpayment-1');

  assert.deepEqual(validateRows(rows), { valid: true, errors: [] });
});

test('validateRows rejects input with more than three columns', () => {
  const rows = parsePastedRows(
    'token-1\textraction-1\tpayment-1\tunexpected-value',
  );

  const result = validateRows(rows);

  assert.equal(result.valid, false);
  assert.equal(result.errors[0].row, 1);
  assert.match(result.errors[0].message, /exactly three columns/i);
});
