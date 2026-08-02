import assert from 'node:assert/strict';
import test from 'node:test';

import { parseTsvRows } from '../public/row-parser.js';

test('parseTsvRows parses exactly three Excel columns', () => {
  assert.deepEqual(parseTsvRows('token\textract\tpayment'), {
    rows: [{
      accessToken: 'token',
      extractionCdk: 'extract',
      paymentCdk: 'payment',
    }],
    errors: [],
  });
});

test('parseTsvRows rejects a fourth TSV column without dropping it', () => {
  assert.deepEqual(parseTsvRows('token\textract\tpayment\tfourth'), {
    rows: [],
    errors: [{ row: 1, message: '应为三列，实际为 4 列' }],
  });
});

test('parseTsvRows rejects a trailing empty TSV column', () => {
  assert.deepEqual(parseTsvRows('token\textract\tpayment\t'), {
    rows: [],
    errors: [{ row: 1, message: '应为三列，实际为 4 列' }],
  });
});
