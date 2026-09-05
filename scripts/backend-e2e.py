#!/usr/bin/env python3
"""Run the real REST journey against a deployed short-term Gateway.

The deployment workflow runs this script from the GitHub runner through the
public API. It intentionally remains compatible with Python 3.6 so the same
journey can still be reproduced from the production host when diagnosing it.
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


def expect_error(response, status, code, label):
    if response.status != status:
        safe_body = response.body.decode("utf-8", errors="replace")[:1000]
        raise RuntimeError(f"{label}: status {response.status}, want {status}: {safe_body}")
    payload = response.json()
    if payload.get("code") != code:
        raise RuntimeError(f"{label}: error code {payload.get('code')!r}, want {code!r}")
    return payload


def assert_no_student_numbers(value, student_numbers, label):
    serialized = json.dumps(value, ensure_ascii=False)
    for student_number in student_numbers:
        if student_number in serialized:
            raise RuntimeError(f"{label}: leaked student number {student_number!r}")


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
    student_observer = f"m6o_{suffix}"
    password = f"M6-{secrets.token_urlsafe(24)}"
    trace_request_id = f"m6trace-{suffix}"

    expect_error(
        client.request("GET", "/api/v1/users/me"),
        401,
        "UNAUTHORIZED",
        "anonymous current user",
    )

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
    observer_auth = expect(
        client.request(
            "POST",
            "/api/v1/auth/register",
            json_body={
                "student_no": student_observer,
                "password": password,
                "nickname": f"M6旁观者{suffix[-4:]}",
            },
        ),
        201,
        "register observer",
    )["data"]
    seller_token = seller_auth["access_token"]
    buyer_token = buyer_auth["access_token"]
    observer_token = observer_auth["access_token"]

    identities = (
        ("seller", seller_auth, student_seller),
        ("buyer", buyer_auth, student_buyer),
        ("observer", observer_auth, student_observer),
    )
    for label, auth, student_number in identities:
        if auth.get("token_type") != "Bearer" or not auth.get("access_token"):
            raise RuntimeError(f"register {label}: invalid bearer token data")
        if auth.get("user", {}).get("student_no") != student_number:
            raise RuntimeError(f"register {label}: owner profile has the wrong student_no")

    seller_me = expect(
        client.request("GET", "/api/v1/users/me", token=seller_token),
        200,
        "get seller profile",
    )["data"]
    if seller_me.get("student_no") != student_seller:
        raise RuntimeError("get seller profile: owner cannot read the expected student_no")

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

    message_key = idempotency_key(args.tag, "message")
    first_message_response = client.request(
        "POST",
        f"/api/v1/conversations/{conversation_id}/messages",
        token=buyer_token,
        json_body={"content": "M6 线上部署验收消息"},
        headers={"Idempotency-Key": message_key},
    )
    first_message = expect(first_message_response, 201, "send message")["data"]
    replay_message_response = client.request(
        "POST",
        f"/api/v1/conversations/{conversation_id}/messages",
        token=buyer_token,
        json_body={"content": "M6 线上部署验收消息"},
        headers={"Idempotency-Key": message_key},
    )
    replay_message = expect(replay_message_response, 201, "replay message")["data"]
    if replay_message.get("id") != first_message.get("id") or replay_message_response.body != first_message_response.body:
        raise RuntimeError("message replay did not preserve the first 201 response body")

    expect_error(
        client.request(
            "GET",
            f"/api/v1/conversations/{conversation_id}/messages",
            token=observer_token,
        ),
        404,
        "RESOURCE_NOT_FOUND",
        "observer list conversation messages",
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

    existing_trade = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/trades",
            token=buyer_token,
            json_body={"conversation_id": conversation_id},
            headers={"Idempotency-Key": idempotency_key(args.tag, "trade-create-existing")},
        ),
        200,
        "create-or-get existing trade",
    )["data"]
    if existing_trade.get("id") != trade_id:
        raise RuntimeError("create-or-get returned a different trade for the same buyer and product")

    expect_error(
        client.request("GET", f"/api/v1/trades/{trade_id}", token=observer_token),
        404,
        "RESOURCE_NOT_FOUND",
        "observer get trade",
    )
    expect_error(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/accept",
            token=buyer_token,
            headers={"Idempotency-Key": idempotency_key(args.tag, "buyer-accept")},
        ),
        403,
        "FORBIDDEN",
        "buyer accept seller-only trade",
    )

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

    # 买家评价：交易完成后买家发布一条，与公开的用户评论相互独立。
    expect_error(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/review",
            token=observer_token,
            json_body={"content": "M6 旁观者评价"},
        ),
        404,
        "RESOURCE_NOT_FOUND",
        "observer create trade review",
    )
    expect_error(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/review",
            token=seller_token,
            json_body={"content": "M6 卖家评价"},
        ),
        403,
        "FORBIDDEN",
        "seller create trade review",
    )
    trade_review = expect(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/review",
            token=buyer_token,
            json_body={"content": "M6 买家评价：交易顺利完成"},
        ),
        201,
        "buyer create trade review",
    )["data"]
    if not trade_review.get("id") or trade_review.get("buyer", {}).get("id") != buyer_auth["user"]["id"]:
        raise RuntimeError("trade review response did not expose the review and buyer identities")
    if not trade_review.get("buyer", {}).get("nickname"):
        raise RuntimeError("trade review response did not complete the buyer nickname")
    expect_error(
        client.request(
            "POST",
            f"/api/v1/trades/{trade_id}/review",
            token=buyer_token,
            json_body={"content": "M6 重复评价"},
        ),
        409,
        "TRADE_REVIEW_ALREADY_EXISTS",
        "duplicate trade review",
    )

    # 用户评论对任意已认证用户开放：不要求购买，不限商品状态，同一用户可以
    # 发布多条评论。这里覆盖旁观者、卖家自评、买家及其第二条评论。
    observer_comment = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/comments",
            token=observer_token,
            json_body={"content": "M6 旁观者评论"},
        ),
        201,
        "observer create comment",
    )["data"]
    seller_comment = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/comments",
            token=seller_token,
            json_body={"content": "M6 卖家补充说明"},
        ),
        201,
        "seller create comment on own product",
    )["data"]
    buyer_comment = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/comments",
            token=buyer_token,
            json_body={"content": "M6 买家评论"},
        ),
        201,
        "buyer create comment",
    )["data"]
    buyer_second_comment = expect(
        client.request(
            "POST",
            f"/api/v1/products/{product_id}/comments",
            token=buyer_token,
            json_body={"content": "M6 买家的第二条评论"},
        ),
        201,
        "buyer create second comment",
    )["data"]
    if not all(
        comment.get("user", {}).get("nickname")
        for comment in (observer_comment, seller_comment, buyer_comment, buyer_second_comment)
    ):
        raise RuntimeError("comment responses did not complete the commenter nickname")
    expect_error(
        client.request(
            "POST",
            "/api/v1/products/p_missing/comments",
            token=buyer_token,
            json_body={"content": "M6 不存在的商品"},
        ),
        404,
        "RESOURCE_NOT_FOUND",
        "comment on missing product",
    )
    comments = expect(
        client.request("GET", f"/api/v1/products/{product_id}/comments", token=observer_token),
        200,
        "list comments",
    )["data"]
    if comments.get("total") != 4 or len(comments.get("items") or []) != 4:
        raise RuntimeError("comment list did not return every created comment")
    if comments["items"][0].get("id") != buyer_second_comment.get("id"):
        raise RuntimeError("comment list is not ordered newest first")
    assert_no_student_numbers(
        comments,
        (student_seller, student_buyer, student_observer),
        "comment list",
    )

    detail_response = client.request(
        "GET",
        f"/api/v1/products/{product_id}",
        token=buyer_token,
        headers={"X-Request-Id": f"m6projection-{suffix}"},
    )
    detail = expect(detail_response, 200, "get sold product")["data"]
    if detail.get("status") != "SOLD" or detail.get("is_favorited") is not True:
        raise RuntimeError("product detail did not expose SOLD and is_favorited=true")
    detail_review = detail.get("buyer_review")
    if not isinstance(detail_review, dict) or detail_review.get("id") != trade_review.get("id"):
        raise RuntimeError("sold product detail did not expose the buyer review")
    if not detail_review.get("buyer", {}).get("nickname") or not detail_review.get("content"):
        raise RuntimeError("buyer review on product detail is incomplete")
    if "student_no" in json.dumps(detail.get("seller", {}), ensure_ascii=False):
        raise RuntimeError("product detail leaked seller student_no")
    assert_no_student_numbers(
        detail,
        (student_seller, student_buyer, student_observer),
        "product detail",
    )

    favorites = expect(client.request("GET", "/api/v1/favorites", token=buyer_token), 200, "list favorites")["data"]
    favorite_item = next((item for item in favorites["items"] if item["product"]["id"] == product_id), None)
    if not favorite_item or favorite_item["product"].get("status") != "SOLD":
        raise RuntimeError("favorite projection did not expose current SOLD status")
    assert_no_student_numbers(
        favorite_item,
        (student_seller, student_buyer, student_observer),
        "favorite projection",
    )

    conversations = expect(client.request("GET", "/api/v1/conversations", token=buyer_token), 200, "list conversations")["data"]
    conversation_item = next((item for item in conversations["items"] if item["id"] == conversation_id), None)
    if not conversation_item or conversation_item["product"].get("status") != "SOLD":
        raise RuntimeError("conversation projection did not expose current SOLD status")
    assert_no_student_numbers(
        conversation_item,
        (student_seller, student_buyer, student_observer),
        "conversation projection",
    )

    my_products = expect(
        client.request("GET", "/api/v1/users/me/products", token=seller_token),
        200,
        "list my products",
    )["data"]
    my_item = next((item for item in my_products["items"] if item["id"] == product_id), None)
    if not my_item or my_item.get("buyer_review", {}) is None:
        raise RuntimeError("my products page did not embed the buyer review")
    if my_item["buyer_review"].get("id") != trade_review.get("id"):
        raise RuntimeError("my products page embedded a different buyer review")
    assert_no_student_numbers(
        my_products,
        (student_seller, student_buyer, student_observer),
        "my products page",
    )

    seller_products = expect(
        client.request("GET", f"/api/v1/users/{seller_auth['user']['id']}/products", token=buyer_token),
        200,
        "list seller products",
    )["data"]
    if not any(
        item["id"] == product_id and item["status"] == "SOLD"
        for item in seller_products["items"]
    ):
        raise RuntimeError("public seller product list did not include the sold product")
    assert_no_student_numbers(
        seller_products,
        (student_seller, student_buyer, student_observer),
        "seller products page",
    )

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
        "comment_count": 4,
        "trace_request_id": trace_request_id,
        "status": "COMPLETED",
        "projection_status": "SOLD",
        "stories": {
            "authentication_and_privacy": "passed",
            "marketplace_and_favorite_projection": "passed",
            "conversation_and_message_idempotency": "passed",
            "trade_create_or_get_and_lifecycle": "passed",
            "trade_buyer_review_and_public_listing": "passed",
            "user_comments_on_visible_products": "passed",
            "observer_visibility_and_role_authorization": "passed",
        },
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
        f"trade={result['trade_id']} comments={result['comment_count']} "
        f"status={result['status']} projection={result['projection_status']}"
    )
    print("STORIES_OK " + ",".join(sorted(result["stories"])))
    baseline = result["baseline"]
    print(
        "BASELINE_OBSERVED "
        f"samples={baseline['samples']} p50_ms={baseline['p50_ms']} "
        f"p95_ms={baseline['p95_ms']} max_ms={baseline['max_ms']}"
    )


if __name__ == "__main__":
    main()
