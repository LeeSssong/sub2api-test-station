# Sub2API Custom Delta Inventory

## Blocked (0)

No items require additional migration decisions.

| Delta | Disposition | Replacement/removal proof |
| --- | --- | --- |
| `AppHeader.vue` 联系客服 trigger | Retire | official sidebar menu acceptance |
| `ContactSupportDialog.vue` | Externalize | `/support` page browser acceptance |
| QQ QR embedded frontend asset | Externalize | preserved homepage asset hash |
| support frontend tests | Retire | homepage and end-to-end support tests |
| custom image build args/tag | Retire | official image release contract |
| `custom_menu_items` | Configuration | idempotent admin settings script |
| `support.md` | Configuration | persistent `/app/data/pages/support.md` |
| in-process update/rollback | Configuration | Caddy guard plus release runbook |
| D04 behavior | Externalize | unchanged internal-test-service contract |
| relay operations/reporting | Externalize | unchanged relay-ops contract |
