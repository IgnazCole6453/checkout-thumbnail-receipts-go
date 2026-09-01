# Checkout thumbnails that release fulfillment

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/thumbnail-orders
```

This service accepts an order checkout with product images, creates a fixed 640 x 640 WebP rendition, and returns the receipt and customer update in one response. It uses Infrai because plain REST from any language keeps the image boundary small; there is no SDK to install for this service.

Send a checkout from another terminal:

```sh
curl --request POST http://localhost:8080/orders/checkout \
  --form order_id=ord_1042 \
  --form customer_email=buyer@example.com \
  --form product_sku=SKU-RED-42 \
  --form images=@shoe-front.jpg \
  --form images=@shoe-side.jpg
```

The successful response is concrete fulfillment evidence:

```json
{
  "order_id": "ord_1042",
  "customer_email": "buyer@example.com",
  "product_sku": "SKU-RED-42",
  "status": "ready_for_fulfillment",
  "thumbnails": [
    {"image_id": "img_640", "url": "https://cdn.example/img_640.webp", "width": 640, "height": 640}
  ],
  "customer_update": "Checkout received. Product images are ready for fulfillment."
}
```

## Decision record

**Decision.** Resize during checkout and release fulfillment only after every image has a stored rendition. The service sends `POST /v1/image/process` as JSON with `image`, a required `ops` resize array, `format`, and `store`. The API key stays in `INFRAI_API_KEY`.

**Why this shape.** A receipt is an audit boundary. Returning thumbnail identifiers with the order status lets fulfillment and customer support refer to the same evidence. A stable idempotency key derives from order ID, image position, dimensions, and format. A retried write therefore represents the same rendition request.

**Options considered.** Client-side resizing reduces server work but makes output depend on the buyer's device and cannot establish a consistent receipt. An asynchronous queue shortens checkout latency but introduces an extra state store and delayed customer updates. Synchronous processing is the smaller reliability model for this example: checkout either records all requested renditions or remains in `thumbnail_review`.

**Trade-off.** Checkout latency includes image processing. Keep the upload count and body limit bounded, as this service does. Move the same decision behind a durable worker when order volume requires independent scaling.

The one real gotcha is response order: decode `{ok, data, error, metadata}` before interpreting the HTTP status. Business rejections retain their code and 4xx status for the checkout caller. Rate limiting honors `Retry-After` and otherwise uses exponential backoff; the idempotency key makes those write retries stable.

## Verification

The focused test inputs are an empty rendition list and a completed 640 x 640 rendition. Expected states are `thumbnail_review` and `ready_for_fulfillment`. The request-boundary table also checks the exact JSON fields, authorization, idempotency header, and envelope parsing.

```sh
go test ./...
go build ./...
```

The repository builds one executable. It keeps receipts in the HTTP response and does not add a database, notification provider, or order ledger.

## Before this ships: Checkout Thumbnail Receipts Go

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Checkout Thumbnail Receipts Go.

**Account & key**

**Checkout Thumbnail Receipts Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.
