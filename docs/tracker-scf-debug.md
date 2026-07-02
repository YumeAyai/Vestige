# Tracker SCF Debug Notes

This note records the CloudBase/SCF behavior we rely on for the lightweight event tracker.

## Response Shape

The Go runtime package `github.com/tencentyun/scf-go-lib/cloudfunction` does not understand HTTP by itself. It only:

1. Unmarshals the invoke payload into the handler event type.
2. Calls the handler.
3. JSON-marshals the handler return value into `InvokeResponse.Payload`.

For HTTP access, CloudBase Gateway interprets that payload as an API Gateway style response. The tracker response type is therefore aliased to:

```go
events.APIGatewayResponse
```

Expected JSON shape:

```json
{
  "isBase64Encoded": true,
  "statusCode": 200,
  "headers": {
    "Content-Type": "image/png"
  },
  "body": "base64..."
}
```

For JSON responses, `isBase64Encoded` is false and `body` is a normal JSON string.

## SCF Logs

SCF logs show the raw function return value in fields such as `ret_msg` or `status_msg`.

Seeing this in logs is normal:

```json
{
  "isBase64Encoded": false,
  "statusCode": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "{\"ok\":true}"
}
```

Do not judge HTTP behavior from `ret_msg` alone. Verify the actual client response:

```sh
curl -i 'https://.../jianji/health'
curl -i 'https://.../jianji/img?type=pixel&token=preview'
```

Expected client responses:

```http
content-type: application/json

{"ok":true,"service":"tracker-scf",...}
```

```http
content-type: image/gif

GIF89a...
```

If the client receives the outer `statusCode/headers/body` object as text, the CloudBase HTTP access integration is not unpacking API Gateway responses correctly.

## Route Checks

Use response headers to confirm where the request went:

```http
x-cloudbase-upstream-type: Tencent-SCF
```

means the request reached the Go cloud function.

```http
x-cloudbase-upstream-type: Tencent-COS
```

means the request was served by static hosting/COS, so the URL did not hit SCF.

The public tracking base URL must point at the SCF HTTP access path:

```yaml
client:
  tracking_base_url: "https://xray-7g6vc4y2d2fc01be-1309857796.ap-shanghai.app.tcloudbase.com/jianji"
```

Image URLs must use that base:

```text
/jianji/img?asset=...&token=...
```

## Asset Debug

Normal asset lookup failures return an empty 404 so email image requests do not leak internals.

For manual debugging, add `debug=1`:

```sh
curl -i 'https://.../jianji/img?asset=<asset-name>&token=preview&debug=1'
```

Example debug error:

```json
{
  "asset": "2c27cee9-d1f2-4587-a886-53b4130afcbc.png",
  "error": "json: cannot unmarshal object into Go struct field tcbAssetDoc.width of type int"
}
```

This means the asset document was found, but decoding failed.

## TCB Document Pitfalls

TCB/Mongo may return extended JSON objects instead of plain Go scalar shapes.

Known cases:

```json
{"_id": {"$oid": "..."}}
{"width": {"$numberInt": "176"}}
{"id": {"$numberLong": "1800000000000000000"}}
```

The store code should use `any` for fields that can come back as extended JSON, then normalize them after unmarshalling.

Asset image data is stored as:

```json
{
  "_id": "asset.png",
  "name": "asset.png",
  "content_type": "application/octet-stream",
  "data_base64": "iVBORw0KGgo...",
  "width": {"$numberInt": "176"}
}
```

When `content_type` is empty or `application/octet-stream`, the Go side decodes `data_base64` and detects the real image MIME from bytes, such as `image/png` or `image/jpeg`.

## Quick Local Checks

Run tests:

```sh
GOCACHE=/private/tmp/vestige-go-cache go test ./tracker/internal/model ./tracker/internal/store ./tracker/internal/handler ./tracker/cmd/scf
```

Build the cloud function package:

```sh
sh scripts/build-tracker-function.sh
```

Output:

```text
dist/main
dist/tracker.zip
```
