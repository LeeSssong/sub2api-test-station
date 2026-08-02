const FIELD_LABELS = [
  ['accessToken', 'access token'],
  ['extractionCdk', 'extraction CDK'],
  ['paymentCdk', 'payment CDK'],
];
const PARSED_COLUMN_COUNTS = new WeakMap();

export function parsePastedRows(text) {
  return String(text ?? '')
    .split(/\r?\n/)
    .filter((line) => line.trim() !== '')
    .map((line) => {
      const delimiter = line.includes('\t') ? '\t' : ',';
      const values = line
        .split(delimiter)
        .map((value) => value.trim());
      const [accessToken = '', extractionCdk = '', paymentCdk = ''] = values;
      const row = { accessToken, extractionCdk, paymentCdk };

      PARSED_COLUMN_COUNTS.set(row, values.length);
      return row;
    });
}

export function validateRows(rows) {
  const errors = [];

  rows.forEach((row, index) => {
    if (PARSED_COLUMN_COUNTS.get(row) > FIELD_LABELS.length) {
      errors.push({
        row: index + 1,
        message: 'Expected exactly three columns',
      });
      return;
    }

    const missingFields = FIELD_LABELS.filter(
      ([field]) => typeof row[field] !== 'string' || row[field].trim() === '',
    ).map(([, label]) => label);

    if (missingFields.length > 0) {
      errors.push({
        row: index + 1,
        message: `Missing ${missingFields.join(', ')}`,
      });
    }
  });

  return { valid: errors.length === 0, errors };
}
