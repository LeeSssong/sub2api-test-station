(() => {
  const form = document.querySelector("#cash-event-form");
  const status = document.querySelector("#cash-event-status");
  if (!form || !status) return;

  const newIdempotencyKey = () => {
    if (globalThis.crypto?.randomUUID) {
      return `accounting:${globalThis.crypto.randomUUID()}`;
    }
    const bytes = new Uint8Array(24);
    globalThis.crypto.getRandomValues(bytes);
    return `accounting:${Array.from(bytes, byte => byte.toString(16).padStart(2, "0")).join("")}`;
  };

  form.addEventListener("submit", async event => {
    event.preventDefault();
    const submit = form.querySelector("button[type=submit]");
    const values = new FormData(form);
    const paidAt = String(values.get("paid_at") || "");
    const accountID = String(values.get("account_id") || "").trim();
    const idempotencyKey = form.dataset.idempotencyKey || newIdempotencyKey();
    form.dataset.idempotencyKey = idempotencyKey;

    const payload = {
      event_type: String(values.get("event_type") || ""),
      paid_at: `${paidAt}:00+08:00`,
      amount_cny: String(values.get("amount_cny") || ""),
      source_kind: String(values.get("source_kind") || ""),
      account_id: accountID === "" ? null : Number(accountID),
      notes: String(values.get("notes") || "")
    };

    submit.disabled = true;
    status.textContent = "正在记录…";
    try {
      const response = await fetch("/relay-ops/api/accounting/cash-events", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": idempotencyKey
        },
        body: JSON.stringify(payload)
      });
      if (!response.ok) {
        status.textContent = response.status === 400
          ? "内容未通过校验，请检查金额、时间、备注和账号 ID。"
          : "记录失败，请稍后重试。";
        return;
      }
      delete form.dataset.idempotencyKey;
      status.textContent = response.status === 201 ? "现金事件已记录。" : "该现金事件已存在。";
      globalThis.location.assign(`/relay-ops/accounting?date=${encodeURIComponent(paidAt.slice(0, 10))}`);
    } catch {
      status.textContent = "网络中断，可直接重试；本次幂等标识会保持不变。";
    } finally {
      submit.disabled = false;
    }
  });
})();
