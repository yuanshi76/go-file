#!/usr/bin/env python3
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "mcp>=1.2.0",
#     "httpx>=0.27.0",
# ]
# ///
"""go-file MCP server — 把 go-file 的 /api/ai/* REST 接口包装成一个 stdio MCP server。

用 uv 运行（推荐，依赖自动安装，无需 pip / venv）：
    uv run --script gofile_mcp.py

为弱模型（如 MiniMax-M2.5 + Hermes）调优：工具精简、单步可完成、按文件名即可操作，
返回里带中文 summary 提示。下载会把文件落到本地磁盘并返回路径。

依赖：
    pip install -r requirements.txt        # mcp>=1.2, httpx

配置（环境变量）：
    GOFILE_BASE_URL       go-file 部署地址，如 http://1.2.3.4:3000（必填）
    GOFILE_TOKEN          用户设置页生成的 Token（必填）
    GOFILE_DOWNLOAD_DIR   下载保存目录，默认当前工作目录下的 ./gofile-downloads
    GOFILE_TIMEOUT        单请求超时秒数，默认 60（上传/下载大文件可调大）
    GOFILE_VERIFY_TLS     https 自签证书时设为 "false" 关闭校验，默认 true

直接运行（stdio）：
    GOFILE_BASE_URL=http://1.2.3.4:3000 GOFILE_TOKEN=xxx python gofile_mcp.py

接入 MCP 客户端（如 Claude Desktop / 任意支持 stdio MCP 的框架），在其
mcpServers 配置里加：
    {
      "mcpServers": {
        "gofile": {
          "command": "python",
          "args": ["/绝对路径/go-file/mcp-server/gofile_mcp.py"],
          "env": {
            "GOFILE_BASE_URL": "http://1.2.3.4:3000",
            "GOFILE_TOKEN": "在用户设置页生成的token"
          }
        }
      }
    }
"""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path
from typing import Any
from urllib.parse import unquote

import httpx
from mcp.server.fastmcp import FastMCP

# --------------------------------------------------------------------------- #
# 配置：全部来自环境变量，绝不硬编码密钥。
# --------------------------------------------------------------------------- #


def _require_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        sys.stderr.write(f"[gofile-mcp] 缺少必填环境变量 {name}\n")
        sys.exit(1)
    return value


BASE_URL = _require_env("GOFILE_BASE_URL").rstrip("/")
TOKEN = _require_env("GOFILE_TOKEN")
DOWNLOAD_DIR = Path(
    os.environ.get("GOFILE_DOWNLOAD_DIR", "").strip() or "./gofile-downloads"
).expanduser()
TIMEOUT = float(os.environ.get("GOFILE_TIMEOUT", "60"))
VERIFY_TLS = os.environ.get("GOFILE_VERIFY_TLS", "true").strip().lower() != "false"

AUTH_HEADERS = {"Authorization": TOKEN}

mcp = FastMCP("gofile")


# --------------------------------------------------------------------------- #
# 内部辅助
# --------------------------------------------------------------------------- #


def _client() -> httpx.Client:
    return httpx.Client(
        base_url=BASE_URL,
        headers=AUTH_HEADERS,
        timeout=TIMEOUT,
        verify=VERIFY_TLS,
        follow_redirects=True,
    )


def _ok(data: Any, summary: str = "") -> str:
    """统一成功返回：JSON 字符串，含给模型的中文 summary 提示。"""
    payload: dict[str, Any] = {"success": True, "data": data}
    if summary:
        payload["summary"] = summary
    return json.dumps(payload, ensure_ascii=False, indent=2)


def _err(message: str) -> str:
    """统一错误返回：把失败原因清楚地告诉模型，便于其决定下一步。"""
    return json.dumps(
        {"success": False, "summary": message, "error": message},
        ensure_ascii=False,
        indent=2,
    )


def _unwrap(resp: httpx.Response) -> Any:
    """解析 go-file 的 {success, message, data} 信封，失败时抛出可读异常。"""
    resp.raise_for_status()
    body = resp.json()
    if isinstance(body, dict) and body.get("success") is False:
        raise RuntimeError(body.get("message") or "服务端返回失败")
    if isinstance(body, dict) and "data" in body:
        return body["data"]
    return body


_FILENAME_STAR_RE = re.compile(r"filename\*\s*=\s*[^']*''([^;]+)", re.IGNORECASE)
_FILENAME_RE = re.compile(r'filename\s*=\s*"?([^";]+)"?', re.IGNORECASE)


def _filename_from_disposition(header: str, fallback: str) -> str:
    """从 Content-Disposition 解析文件名，优先 RFC 5987 的 filename*。"""
    if header:
        m = _FILENAME_STAR_RE.search(header)
        if m:
            return unquote(m.group(1)).strip()
        m = _FILENAME_RE.search(header)
        if m:
            return m.group(1).strip()
    return fallback


def _safe_name(name: str) -> str:
    """去掉路径分隔符等危险字符，避免下载写出到目录外。"""
    name = name.replace("\\", "/").split("/")[-1]
    name = re.sub(r'[<>:"|?*\x00-\x1f]', "_", name).strip().strip(".")
    return name or "download.bin"


def _unique_path(directory: Path, name: str) -> Path:
    """同名文件追加 (1)/(2) 后缀，避免覆盖。"""
    target = directory / name
    if not target.exists():
        return target
    stem, suffix = target.stem, target.suffix
    i = 1
    while True:
        candidate = directory / f"{stem} ({i}){suffix}"
        if not candidate.exists():
            return candidate
        i += 1


# --------------------------------------------------------------------------- #
# 工具
# --------------------------------------------------------------------------- #


@mcp.tool()
def find_files(query: str, limit: int = 5) -> str:
    """按文件名或关键词查找文件，返回最匹配的几条（含 id、filename、大小、download_url）
    和一句中文 summary。当你不确定文件是否存在、或要从多个候选里确认时使用；确认后可直接
    download_file。query 例：\"发票\"、\"2026 预算\"。limit 默认 5，最大 20。"""
    try:
        with _client() as client:
            data = _unwrap(
                client.get("/api/ai/find", params={"q": query, "limit": limit})
            )
        items = data.get("items", []) if isinstance(data, dict) else []
        summary = (data.get("summary") if isinstance(data, dict) else "") or (
            f"找到 {len(items)} 个匹配 “{query}” 的文件。"
            if items
            else f"没有找到匹配 “{query}” 的文件。"
        )
        return _ok(data, summary)
    except Exception as exc:  # noqa: BLE001 — 把任何失败原因回传给模型
        return _err(f"查找失败：{exc}")


@mcp.tool()
def download_file(query: str, save_dir: str = "") -> str:
    """下载一个文件到本地磁盘。query 可以是文件名（如 \"季度报告.pdf\"）、关键词，或文件的
    数字 id；服务端会自动找到最匹配的文件。这是下载的首选方式——不需要先知道 id。下载成功后
    返回保存的本地绝对路径与文件大小。save_dir 可选，留空则保存到 GOFILE_DOWNLOAD_DIR。"""
    try:
        directory = Path(save_dir).expanduser() if save_dir.strip() else DOWNLOAD_DIR
        directory.mkdir(parents=True, exist_ok=True)
        with _client() as client:
            with client.stream("GET", "/api/ai/download", params={"q": query}) as resp:
                if resp.status_code == 404:
                    return _err(f"没有找到匹配 “{query}” 的文件，可先用 find_files 确认。")
                resp.raise_for_status()
                name = _safe_name(
                    _filename_from_disposition(
                        resp.headers.get("Content-Disposition", ""),
                        fallback=_safe_name(query) or "download.bin",
                    )
                )
                path = _unique_path(directory, name)
                total = 0
                with open(path, "wb") as fh:
                    for chunk in resp.iter_bytes():
                        fh.write(chunk)
                        total += len(chunk)
        return _ok(
            {"saved_path": str(path.resolve()), "filename": name, "bytes": total},
            summary=f"已下载 “{name}”（{total} 字节）到 {path.resolve()}。",
        )
    except Exception as exc:  # noqa: BLE001
        return _err(f"下载失败：{exc}")


@mcp.tool()
def upload_file(file_path: str, description: str = "") -> str:
    """上传/回传一个本地文件到暂存站。file_path 为本地文件完整路径，例：\"/tmp/result.csv\"。
    description 可选，对文件的简短说明。成功后返回该文件的 id 与 download_url。"""
    try:
        src = Path(file_path).expanduser()
        if not src.is_file():
            return _err(f"本地文件不存在：{file_path}")
        with _client() as client, open(src, "rb") as fh:
            data = _unwrap(
                client.post(
                    "/api/ai/files",
                    files={"file": (src.name, fh, "application/octet-stream")},
                    data={"description": description} if description else None,
                )
            )
        items = data.get("items", []) if isinstance(data, dict) else []
        return _ok(
            data,
            summary=f"已上传 “{src.name}”。"
            + (f" id={items[0].get('id')}" if items else ""),
        )
    except Exception as exc:  # noqa: BLE001
        return _err(f"上传失败：{exc}")


@mcp.tool()
def list_files(query: str = "", page: int = 1, page_size: int = 20) -> str:
    """分页列出 / 搜索文件。query 为空则列出全部（按上传时间倒序）。page 默认 1，
    page_size 默认 20、最大 100。返回 items 列表与 total/page/page_size。"""
    try:
        params: dict[str, Any] = {"page": page, "page_size": page_size}
        if query.strip():
            params["q"] = query
        with _client() as client:
            data = _unwrap(client.get("/api/ai/files", params=params))
        total = data.get("total") if isinstance(data, dict) else None
        return _ok(data, summary=f"共 {total} 个文件，当前第 {page} 页。")
    except Exception as exc:  # noqa: BLE001
        return _err(f"列出文件失败：{exc}")


@mcp.tool()
def get_stats() -> str:
    """获取暂存站整体统计：文件总数、总占用、总下载次数、各类型分布、归档/本地分布等。
    当用户问\"有多少文件 / 占用多大 / 最常下载的是哪些\"时使用。"""
    try:
        with _client() as client:
            data = _unwrap(client.get("/api/ai/stats"))
        summary = ""
        if isinstance(data, dict):
            summary = (
                f"共 {data.get('total_files')} 个文件，"
                f"总占用 {data.get('total_size')} 字节，"
                f"总下载 {data.get('total_downloads')} 次。"
            )
        return _ok(data, summary)
    except Exception as exc:  # noqa: BLE001
        return _err(f"获取统计失败：{exc}")


def main() -> None:
    """控制台入口（pyproject 的 gofile-mcp 脚本）。默认 stdio 传输。"""
    mcp.run()


if __name__ == "__main__":
    main()
