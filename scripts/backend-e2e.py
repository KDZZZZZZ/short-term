#!/usr/bin/env python3
"""Run the real REST journey against a deployed short-term Gateway.

This script intentionally remains compatible with Python 3.6, which is the
version available on the production host.
"""

import argparse
import json
import math
import secrets
import statistics
import time
import urllib.error
import urllib.request


class Response:
    def __init__(self, status, headers, body, elapsed_ms):
        self.status = status
        self.headers = headers
        self.body = body
        self.elapsed_ms = elapsed_ms

    def json(self):
        return json.loads(self.body.decode("utf-8"))


class Client:
    def __init__(self, base_url, timeout):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def request(
        self,
        method,
        path,
        *,
        token=None,
        json_body=None,
        form=None,
        headers=None,
    ):
        request_headers = {"Accept": "application/json"}
        if token:
            request_headers["Authorization"] = f"Bearer {token}"
        if headers:
            request_headers.update(headers)

        data = None
        if json_body is not None:
            data = json.dumps(json_body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            request_headers["Content-Type"] = "application/json"
        elif form is not None:
            boundary = f"shortterm-{secrets.token_hex(12)}"
            chunks = []
            for name, value in form.items():
                chunks.extend(
                    [
                        f"--{boundary}\r\n".encode(),
                        f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode(),
                        value.encode("utf-8"),
                        b"\r\n",
                    ]
                )
            chunks.append(f"--{boundary}--\r\n".encode())
            data = b"".join(chunks)
            request_headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"

        request = urllib.request.Request(
            self.base_url + path,
            data=data,
            headers=request_headers,
            method=method,
        )
        started = time.perf_counter()
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                body = response.read()
                return Response(response.status, response.headers, body, (time.perf_counter() - started) * 1000)
        except urllib.error.HTTPError as error:
            return Response(error.code, error.headers, error.read(), (time.perf_counter() - started) * 1000)


def expect(response, status, label):
    if response.status != status:
        safe_body = response.body.decode("utf-8", errors="replace")[:1000]
        raise RuntimeError(f"{label}: status {response.status}, want {status}: {safe_body}")
    payload = response.json()
    if payload.get("code") != "OK":
        raise RuntimeError(f"{label}: unexpected envelope code {payload.get('code')!r}")
    return payload


def idempotency_key(tag, operation):
    return f"m6-{tag[:24]}-{operation}-{secrets.token_hex(8)}"[:128]


def percentile(values, fraction):
    ordered = sorted(values)
    index = max(0, math.ceil(len(ordered) * fraction) - 1)
    return ordered[index]


def run(args):
    client = Client(args.base_url, args.timeout)
    suffix = secrets.token_hex(6)
    student_seller = f"m6s_{suffix}"
    student_buyer = f"m6b_{suffix}"
    password = f"M6-{secrets.token_urlsafe(24)}"
    trace_request_id = f"m6trace-{suffix}"

    seller_auth = expect(
        client.request(
            "POST",
            "/api/v1/auth/register",
            json_body={
                "student_no": student_seller,
                "password": password,
                "nickname": f"M6卖家{suffix[-4:]}",
                "wechat": f"m6seller_{suffix}",
            },
        ),
        201,
        "register seller",
    )["data"]
    buyer_auth = expect(
        client.request(
            "POST",
            "/api/v1/auth/register",
            json_body={
                "student_no": student_buyer,
                "password": password,
                "nickname": f"M6买家{suffix[-4:]}",
            },
        ),
        201,
        "register buyer",
    )["data"]
    seller_token = seller_auth["access_token"]
    buyer_token = buyer_auth["access_token"]

    product = expect(
        client.request(
            "POST",
            "/api/v1/products",
            token=seller_token,
            form={
                "title": f"M6线上验收商品 {args.tag[:12]} {suffix}",
                "price": "88.80",
                "category": "OTHER",
                "description": "自动部署完成后的真实生产链路验收数据。",
            },
        ),
        201,
        "create product",
    )["data"]
    product_id = product["id"]
    if product.get("status") != "ON_SALE":
        raise RuntimeError("create product: initial status is not ON_SALE")

    expect(
        client.request("PUT", f"/api/v1/favorites/{product_id}", token=buyer_token),
        200,
        "favorite product",
    )

    conversation = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/conversations",
            token=buyer_token,
            headers={"Idempotency-Key": idempotency_key(args.tag, "conversation")},
        ),
        200,
        "create conversation",
    )["data"]
    conversation_id = conversation["id"]

    expect(
        client.request(
            "POST",
            f"/api/v1/conversations/{conversation_id}/messages",
            token=buyer_token,
            json_body={"content": "M6 线上部署验收消息"},
            headers={"Idempotency-Key": idempotency_key(args.tag, "message")},
        ),
        201,
        "send message",
    )

    trade_key = idempotency_key(args.tag, "trade-create")
    first_trade_response = client.request(
        "POST",
        f"/api/v1/products/{product_id}/trades",
        token=buyer_token,
        json_body={"conversation_id": conversation_id},
        headers={"Idempotency-Key": trade_key, "X-Request-Id": trace_request_id},
    )
    first_trade = expect(first_trade_response, 201, "create trade")["data"]
    replay_response = client.request(
        "POST",
        f"/api/v1/products/{product_id}/trades",
        token=buyer_token,
        json_body={"conversation_id": conversation_id},
        headers={"Idempotency-Key": trade_key},
    )
    replay_trade = expect(replay_response, 201, "replay trade")["data"]
    if replay_trade["id"] != first_trade["id"] or replay_response.body != first_trade_response.body:
        raise RuntimeError("trade replay did not preserve the first 201 response body")
    trade_id = first_trade["id"]

    expect(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/accept",
            token=seller_token,
            headers={"Idempotency-Key": idempotency_key(args.tag, "accept")},
        ),
        200,
        "accept trade",
    )
    expect(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/confirm",
            token=seller_token,
            headers={"Idempotency-Key": idempotency_key(args.tag, "seller-confirm")},
        ),
        200,
        "seller confirm",
    )
    completed_trade = expect(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/confirm",
            token=buyer_token,
            headers={"Idempotency-Key": idempotency_key(args.tag, "buyer-confirm")},
        ),
        200,
        "buyer confirm",
    )["data"]
    if completed_trade.get("status") != "COMPLETED" or completed_trade.get("product", {}).get("status") != "SOLD":
        raise RuntimeError("completed trade did not move the product to SOLD")

    detail_response = client.request(
        "GET",
        f"/api/v1/products/{product_id}",
        token=buyer_token,
        headers={"X-Request-Id": f"m6projection-{suffix}"},
    )
    detail = expect(detail_response, 200, "get sold product")["data"]
    if detail.get("status") != "SOLD" or detail.get("is_favorited") is not True:
        raise RuntimeError("product detail did not expose SOLD and is_favorited=true")
    if "student_no" in json.dumps(detail.get("seller", {}), ensure_ascii=False):
        raise RuntimeError("product detail leaked seller student_no")

    favorites = expect(client.request("GET", "/api/v1/favorites", token=buyer_token), 200, "list favorites")["data"]
    favorite_item = next((item for item in favorites["items"] if item["product"]["id"] == product_id), None)
    if not favorite_item or favorite_item["product"].get("status") != "SOLD":
        raise RuntimeError("favorite projection did not expose current SOLD status")

    conversations = expect(client.request("GET", "/api/v1/conversations", token=buyer_token), 200, "list conversations")["data"]
    conversation_item = next((item for item in conversations["items"] if item["id"] == conversation_id), None)
    if not conversation_item or conversation_item["product"].get("status") != "SOLD":
        raise RuntimeError("conversation projection did not expose current SOLD status")

    latencies = []
    for _ in range(args.baseline_samples):
        response = client.request("GET", "/api/v1/products?page=1&page_size=20", token=buyer_token)
        expect(response, 200, "baseline list products")
        latencies.append(response.elapsed_ms)

    result = {
        "tag": args.tag,
        "product_id": product_id,
        "conversation_id": conversation_id,
        "trade_id": trade_id,
        "trace_request_id": trace_request_id,
        "status": "COMPLETED",
        "projection_status": "SOLD",
        "baseline": {
            "path": "GET /api/v1/products?page=1&page_size=20",
            "samples": len(latencies),
            "p50_ms": round(statistics.median(latencies), 2),
            "p95_ms": round(percentile(latencies, 0.95), 2),
            "max_ms": round(max(latencies), 2),
        },
    }
    if args.result_file:
        with open(args.result_file, "w", encoding="utf-8") as output:
            json.dump(result, output, ensure_ascii=False, indent=2)
            output.write("\n")
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--timeout", type=float, default=10.0)
    parser.add_argument("--baseline-samples", type=int, default=20)
    parser.add_argument("--result-file")
    args = parser.parse_args()
    if args.baseline_samples < 1 or args.baseline_samples > 200:
        parser.error("--baseline-samples must be between 1 and 200")

    result = run(args)
    print(
        "E2E_OK "
        f"product={result['product_id']} conversation={result['conversation_id']} "
        f"trade={result['trade_id']} status={result['status']} projection={result['projection_status']}"
    )
    baseline = result["baseline"]
    print(
        "BASELINE_OBSERVED "
        f"samples={baseline['samples']} p50_ms={baseline['p50_ms']} "
        f"p95_ms={baseline['p95_ms']} max_ms={baseline['max_ms']}"
    )


if __name__ == "__main__":
    main()
