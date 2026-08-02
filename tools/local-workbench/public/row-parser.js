export function parseTsvRows(text) {
  const rows = [];
  const errors = [];
  const lines = String(text ?? '').split(/\r?\n/);

  lines.forEach((line, index) => {
    if (line.trim() === '') {
      return;
    }

    const values = line.split('\t');
    if (values.length !== 3) {
      errors.push({
        row: index + 1,
        message: `应为三列，实际为 ${values.length} 列`,
      });
      return;
    }

    const [accessToken, extractionCdk, paymentCdk] = values;
    rows.push({
      accessToken: accessToken.trim(),
      extractionCdk: extractionCdk.trim(),
      paymentCdk: paymentCdk.trim(),
    });
  });

  return { rows: errors.length === 0 ? rows : [], errors };
}
