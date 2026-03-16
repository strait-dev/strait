"""HTTP request execution for the Strait SDK."""

from __future__ import annotations

import re
from typing import Any

import httpx

from strait._config import Config, get_authorization_header
from strait._errors import DecodeError, TransportError, map_http_error


def substitute_path_params(path: str, params: dict[str, str]) -> str:
    """Replace {param} placeholders in a path with their values."""
    def _replace(match: re.Match[str]) -> str:
        key = match.group(1)
        return params.get(key, match.group(0))

    return re.sub(r"\{(\w+)\}", _replace, path)


def do_request(
    http_client: httpx.Client,
    config: Config,
    *,
    method: str,
    path: str,
    path_params: dict[str, str] | None = None,
    query: dict[str, str] | None = None,
    headers: dict[str, str] | None = None,
    body: Any = None,
) -> dict[str, Any]:
    """Execute a synchronous HTTP request."""
    if path_params:
        path = substitute_path_params(path, path_params)

    url = config.base_url + path

    req_headers: dict[str, str] = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "Authorization": get_authorization_header(config.auth),
    }
    req_headers.update(config.default_headers)
    if headers:
        req_headers.update(headers)

    try:
        response = http_client.request(
            method,
            url,
            params=query,
            headers=req_headers,
            json=body if body is not None else None,
            content=None if body is not None else None,
        )
    except httpx.HTTPError as exc:
        raise TransportError(f"request failed: {exc}", cause=exc)

    resp_body = response.text

    if response.status_code < 200 or response.status_code >= 300:
        err_body: Any = None
        try:
            err_body = response.json()
        except Exception:
            pass

        msg = f"HTTP {response.status_code}: {method} {path}"
        if isinstance(err_body, dict) and "message" in err_body:
            msg = err_body["message"]

        raise map_http_error(response.status_code, msg, err_body)

    if not resp_body:
        return {}

    try:
        return response.json()  # type: ignore[no-any-return]
    except Exception as exc:
        raise DecodeError(
            f"failed to decode response: {exc}",
            body=resp_body,
            cause=exc,
        )


async def do_request_async(
    http_client: httpx.AsyncClient,
    config: Config,
    *,
    method: str,
    path: str,
    path_params: dict[str, str] | None = None,
    query: dict[str, str] | None = None,
    headers: dict[str, str] | None = None,
    body: Any = None,
) -> dict[str, Any]:
    """Execute an asynchronous HTTP request."""
    if path_params:
        path = substitute_path_params(path, path_params)

    url = config.base_url + path

    req_headers: dict[str, str] = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "Authorization": get_authorization_header(config.auth),
    }
    req_headers.update(config.default_headers)
    if headers:
        req_headers.update(headers)

    try:
        response = await http_client.request(
            method,
            url,
            params=query,
            headers=req_headers,
            json=body if body is not None else None,
            content=None if body is not None else None,
        )
    except httpx.HTTPError as exc:
        raise TransportError(f"request failed: {exc}", cause=exc)

    resp_body = response.text

    if response.status_code < 200 or response.status_code >= 300:
        err_body: Any = None
        try:
            err_body = response.json()
        except Exception:
            pass

        msg = f"HTTP {response.status_code}: {method} {path}"
        if isinstance(err_body, dict) and "message" in err_body:
            msg = err_body["message"]

        raise map_http_error(response.status_code, msg, err_body)

    if not resp_body:
        return {}

    try:
        return response.json()  # type: ignore[no-any-return]
    except Exception as exc:
        raise DecodeError(
            f"failed to decode response: {exc}",
            body=resp_body,
            cause=exc,
        )
