#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RENDER="python3 $HERE/render.py"

BASE="${BASE:-http://localhost:8080}"
API="$BASE/api/v1"
KDS="${KDS:-http://localhost:8081}"

HINKALNAYA="11111111-1111-4111-8111-111111111111"
PIZZA="22222222-2222-4222-8222-222222222222"
SUSHI="33333333-3333-4333-8333-333333333333"
PIZZA_KEY="demo-pizza-4c81e2b7a95d0f36"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
step() { printf '\n\033[1;36m> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mOK\033[0m %s\n' "$*"; }
info() { printf '    %s\n' "$*"; }
fail() { printf '  \033[31mFAILED: %s\033[0m\n' "$*"; exit 1; }

command -v curl    >/dev/null || fail "curl is required"
command -v python3 >/dev/null || fail "python3 is required"

new_customer() { cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr 'A-Z' 'a-z'; }

get() { local path="$1"; shift; curl -sS "$API$path" "$@"; }
send() {
  local method="$1" path="$2" body="$3"; shift 3
  curl -sS -X "$method" "$API$path" -H 'Content-Type: application/json' -d "$body" "$@"
}
BODY_FILE=/tmp/avito_kitchen_demo.json
send_status() {
  local method="$1" path="$2" body="$3"; shift 3
  curl -sS -o "$BODY_FILE" -w '%{http_code}' -X "$method" "$API$path" \
    -H 'Content-Type: application/json' -d "$body" "$@"
}
body_code() { $RENDER field code < "$BODY_FILE"; }

bold "Avito.Kitchen - end-to-end walkthrough"

step "0. The platform is up"
curl -fsS "$BASE/readyz" >/dev/null || fail "the API is not answering - run 'task up' first"
ok "API is ready"

menu_sections() { get "/venues/$HINKALNAYA/menu" 2>/dev/null | $RENDER count sections 2>/dev/null || echo 0; }
for attempt in $(seq 1 60); do
  [ "$(menu_sections)" -ge 2 ] && break
  [ "$attempt" -eq 60 ] && fail "the example venue never published its menu - check 'docker compose logs partner-demo'"
  sleep 1
done
ok "the example venue has published its menu through the partner API"

CUSTOMER="$(new_customer)"
info "acting as customer $CUSTOMER"

step "1. The customer browses the storefront"
get "/venues" | $RENDER venues
ok "closed venues stay visible instead of disappearing from the list"

step "2. ...opens Хинкальная №1 and reads the menu"
MENU="$(get "/venues/$HINKALNAYA/menu")"
echo "$MENU" | $RENDER menu
ITEM_A="$(echo "$MENU" | $RENDER field sections.0.items.0.id)"
ITEM_B="$(echo "$MENU" | $RENDER field sections.1.items.0.id)"
ok "availability is on the menu, so nothing is discovered at checkout"

step "3. ...fills a cart"
send PUT "/cart/items" \
  "{\"venue_id\":\"$HINKALNAYA\",\"menu_item_id\":\"$ITEM_A\",\"quantity\":2}" \
  -H "X-Customer-Id: $CUSTOMER" >/dev/null
send PUT "/cart/items" \
  "{\"venue_id\":\"$HINKALNAYA\",\"menu_item_id\":\"$ITEM_B\",\"quantity\":1}" \
  -H "X-Customer-Id: $CUSTOMER" | $RENDER cart
ok "one boolean tells the client whether to enable the checkout button"

step "4. ...tries to add a pizza to a cart full of хинкали"
PIZZA_MENU="$(get "/venues/$PIZZA/menu")"
PIZZA_ITEM="$(echo "$PIZZA_MENU" | $RENDER item-id in_stock)"
CODE="$(send_status PUT "/cart/items" \
  "{\"venue_id\":\"$PIZZA\",\"menu_item_id\":\"$PIZZA_ITEM\",\"quantity\":1}" \
  -H "X-Customer-Id: $CUSTOMER")"
info "HTTP $CODE  $(body_code)"
[[ "$CODE" == "409" ]] || fail "a cart must stay bound to one venue"
ok "one cart, one kitchen - raised when the dish is added, not at checkout"

step "5. ...checks out"
IDEMPOTENCY="demo-$CUSTOMER"
ORDER="$(send POST "/orders" \
  '{"delivery":{"recipient_name":"Алексей","phone":"+7 900 000-00-00","address":"Москва, ул. Пятницкая, 20, кв. 3","comment":"Домофон не работает"}}' \
  -H "X-Customer-Id: $CUSTOMER" -H "Idempotency-Key: $IDEMPOTENCY")"
echo "$ORDER" | $RENDER order
ORDER_ID="$(echo "$ORDER" | $RENDER field id)"
ok "order, stock reservation and venue notification committed together"

step "6. ...and the request is retried after a flaky connection"
RETRY_ID="$(send POST "/orders" \
  '{"delivery":{"recipient_name":"Алексей","phone":"+7 900 000-00-00","address":"Москва, ул. Пятницкая, 20, кв. 3"}}' \
  -H "X-Customer-Id: $CUSTOMER" -H "Idempotency-Key: $IDEMPOTENCY" | $RENDER field id)"
[[ "$RETRY_ID" == "$ORDER_ID" ]] || fail "the retry created a second order"
ok "Idempotency-Key returned the original order rather than charging twice"

step "7. The venue's own service picks the order up over gRPC and cooks it"
info "(partner-demo is driving this; watch it live with 'task logs')"
LAST=""
for _ in $(seq 1 20); do
  STATUS="$(get "/orders/$ORDER_ID" -H "X-Customer-Id: $CUSTOMER" | $RENDER field status)"
  if [[ "$STATUS" != "$LAST" ]]; then info "status -> $STATUS"; LAST="$STATUS"; fi
  [[ "$STATUS" == "delivered" ]] && break
  sleep 3
done
[[ "$LAST" == "delivered" ]] || fail "the order never reached 'delivered' - is partner-demo running?"
get "/orders/$ORDER_ID" -H "X-Customer-Id: $CUSTOMER" | $RENDER timeline
ok "the customer sees who moved the order and when"

step "8. A dish that ran out, and one the kitchen switched off"
SOLD_OUT="$(echo "$PIZZA_MENU" | $RENDER item-id sold_out)"
DISABLED="$(echo "$PIZZA_MENU" | $RENDER item-id disabled)"
CUSTOMER2="$(new_customer)"
send PUT "/cart/items" \
  "{\"venue_id\":\"$PIZZA\",\"menu_item_id\":\"$SOLD_OUT\",\"quantity\":1}" \
  -H "X-Customer-Id: $CUSTOMER2" >/dev/null
send PUT "/cart/items" \
  "{\"venue_id\":\"$PIZZA\",\"menu_item_id\":\"$DISABLED\",\"quantity\":1}" \
  -H "X-Customer-Id: $CUSTOMER2" | $RENDER cart-problems
CODE="$(send_status POST "/orders" \
  '{"delivery":{"recipient_name":"Тест","phone":"+79000000001","address":"Москва, Ленинский 1"}}' \
  -H "X-Customer-Id: $CUSTOMER2" -H "Idempotency-Key: demo-unavailable-$CUSTOMER2")"
info "checkout -> HTTP $CODE  $(body_code)"
$RENDER unavailable < "$BODY_FILE"
[[ "$CODE" == "409" ]] || fail "expected 409 items_unavailable"
ok "the client is told exactly which lines to fix, and why each one failed"

step "9. A venue that is closed right now"
SUSHI_ITEM="$(get "/venues/$SUSHI/menu" | $RENDER item-id first)"
CUSTOMER3="$(new_customer)"
send PUT "/cart/items" \
  "{\"venue_id\":\"$SUSHI\",\"menu_item_id\":\"$SUSHI_ITEM\",\"quantity\":4}" \
  -H "X-Customer-Id: $CUSTOMER3" | $RENDER cart-problems
CODE="$(send_status POST "/orders" \
  '{"delivery":{"recipient_name":"Тест","phone":"+79000000002","address":"Москва, Пресненская 1"}}' \
  -H "X-Customer-Id: $CUSTOMER3" -H "Idempotency-Key: demo-closed-$CUSTOMER3")"
info "checkout -> HTTP $CODE  $(body_code)"
[[ "$CODE" == "409" ]] || fail "a closed venue must refuse the order"
ok "a closed kitchen cannot be ordered from"

step "10. The customer cancels, and the portions go back on the menu"
CUSTOMER4="$(new_customer)"
IN_STOCK="$(echo "$PIZZA_MENU" | $RENDER item-id in_stock)"
stock_now() { get "/venues/$PIZZA/menu" | $RENDER stock-of "$IN_STOCK"; }
BEFORE="$(stock_now)"
send PUT "/cart/items" \
  "{\"venue_id\":\"$PIZZA\",\"menu_item_id\":\"$IN_STOCK\",\"quantity\":2}" \
  -H "X-Customer-Id: $CUSTOMER4" >/dev/null
CANCEL_ID="$(send POST "/orders" \
  '{"delivery":{"recipient_name":"Ольга","phone":"+79001112233","address":"Москва, Ленинский 45"}}' \
  -H "X-Customer-Id: $CUSTOMER4" -H "Idempotency-Key: demo-cancel-$CUSTOMER4" | $RENDER field id)"
DURING="$(stock_now)"
send POST "/orders/$CANCEL_ID/cancel" '{"reason":"передумал"}' \
  -H "X-Customer-Id: $CUSTOMER4" >/dev/null
AFTER="$(stock_now)"
info "stock: $BEFORE -> $DURING (reserved) -> $AFTER (released)"
[[ "$AFTER" == "$BEFORE" ]] || fail "cancelling did not return the portions"
ok "a cancellation is not a lost sale for the next customer"

step "11. A venue reaching for another venue's order"
CODE="$(send_status POST "/partner/orders/$ORDER_ID/accept" '{}' -H "X-Api-Key: $PIZZA_KEY")"
info "Pizza Avenue tries to accept a Хинкальная order -> HTTP $CODE  $(body_code)"
[[ "$CODE" == "404" ]] || fail "another venue's order must not even be visible"
ok "the API key scopes a partner to its own data"

step "12. The example venue's own kitchen display"
curl -fsS "$KDS/kds" | $RENDER kds
ok "the venue runs its own service, with its own API and its own policy"

printf '\n\033[1;32mAll scenarios passed.\033[0m\n'
printf '  Swagger UI       http://localhost:8088\n'
printf '  Kitchen display  %s/kds\n\n' "$KDS"
