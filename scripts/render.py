import json
import sys

def rub(money):
    return f"{money['amount'] / 100:,.2f} RUB".replace(",", " ")

def venues(page):
    for venue in page["items"]:
        state = "open" if venue["accepts_orders"] else "CLOSED"
        name = venue["name"]
        print(f"    {name:<22} {state:<7} rating {venue['rating']}"
              f"   min order {rub(venue['min_order_amount'])}")

def menu(document):
    for section in document["sections"]:
        print(f"    {section['category']['name']}")
        for item in section["items"]:
            mark = " " if item["availability"]["orderable"] else "x"
            stock = item["availability"]["stock_quantity"]
            print(f"      {mark} {item['name']:<40} {rub(item['price']):>12}"
                  f"   stock {stock}")

def cart(document):
    for line in document["lines"]:
        print(f"    {line['name']:<40} x{line['quantity']}"
              f" {rub(line['line_total']):>12}")
    print(f"    {'total':<40}    {rub(document['total']):>12}")
    print(f"    checkout_ready: {document['checkout_ready']}")
    if document.get("blockers"):
        for blocker in document["blockers"]:
            print(f"      blocked by: {blocker}")

def cart_problems(document):
    for line in document["lines"]:
        print(f"    {line['name']:<24} orderable={line['orderable']}"
              f"  problem={line.get('problem')}")
    if document.get("blockers"):
        print(f"    blockers: {'; '.join(document['blockers'])}")

def order(document):
    print(f"    order {document['number']}   status {document['status']}"
          f"   total {rub(document['total'])}")

def timeline(document):
    print("    timeline:")
    for entry in document["timeline"]:
        moment = entry["occurred_at"][11:19]
        previous = entry.get("from_status", "-")
        print(f"      {moment}  {previous:>12} -> {entry['status']:<12}"
              f" by {entry['actor']}")

def unavailable(document):
    for item in document.get("unavailable_items", []):
        print(f"      {item['name']:<24} requested {item['requested']},"
              f" available {item['available']}  ({item['reason']})")

def kds(document):
    print(f"    venue:  {document['venue_name']}")
    policy = document["policy"]
    print(f"    policy: auto_accept={policy['auto_accept']}"
          f"  prep={policy['prep_minutes']}min")
    print(f"    orders on the pass: {len(document['orders'])}")

def field(document, path):
    value = document
    for part in path.split("."):
        value = value[int(part)] if part.isdigit() else value[part]
    print(value)

def count(document, path):
    value = document
    for part in path.split("."):
        value = value[int(part)] if part.isdigit() else value[part]
    print(len(value))

def item_id(document, predicate):
    tests = {
        "sold_out": lambda i: i["availability"]["stock_quantity"] == 0,
        "disabled": lambda i: not i["is_available"],
        "first": lambda i: True,
        "in_stock": lambda i: i["availability"]["stock_quantity"] > 3,
    }
    test = tests[predicate]
    for section in document["sections"]:
        for item in section["items"]:
            if test(item):
                print(item["id"])
                return
    sys.exit(f"no menu item matching {predicate!r}")

def stock_of(document, wanted_id):
    for section in document["sections"]:
        for item in section["items"]:
            if item["id"] == wanted_id:
                print(item["availability"]["stock_quantity"])
                return
    sys.exit(f"item {wanted_id} not found")

VIEWS = {
    "venues": venues,
    "menu": menu,
    "cart": cart,
    "cart-problems": cart_problems,
    "order": order,
    "timeline": timeline,
    "unavailable": unavailable,
    "kds": kds,
    "field": field,
    "count": count,
    "item-id": item_id,
    "stock-of": stock_of,
}

def main():
    if len(sys.argv) < 2 or sys.argv[1] not in VIEWS:
        sys.exit(f"usage: {sys.argv[0]} <{'|'.join(VIEWS)}> [args]")
    document = json.load(sys.stdin)
    VIEWS[sys.argv[1]](document, *sys.argv[2:])

if __name__ == "__main__":
    main()
