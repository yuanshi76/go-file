<p align="center">
  <a href="https://github.com/songquanpeng/go-file"><img src="https://user-images.githubusercontent.com/39998050/108494937-1a573e80-72e3-11eb-81c3-5545d7c2ed6e.jpg" width="200" height="200" alt="go-file"></a>
</p>

<div align="center">

# Go File（云部署强化分支）

_✨ 文件分享工具，单可执行文件开箱即用；本分支在原版基础上做了**安全加固**与**OSS 冷存储归档**，更适合公网 / 云端长期部署 ✨_

</div>

<p align="center">
  <a href="#本分支相比原版的改进">分支改进</a>
  ·
  <a href="#环境变量一览">环境变量</a>
  ·
  <a href="#使用-docker-compose-部署推荐">Compose 部署</a>
  ·
  <a href="#冷存储归档oss--webdav">冷存储归档</a>
  ·
  <a href="#ai-可调用-apiai-适配">AI API</a>
  ·
  <a href="#演示">截图展示</a>
</p>

> **关于本分支**：Fork 自 [songquanpeng/go-file](https://github.com/songquanpeng/go-file)，核心交互与原版一致。本分支聚焦**公网/云端部署**场景，补齐了安全短板，并新增「长期不访问的文件自动归档到阿里云 OSS、按需经 WebDAV 取回」的冷存储能力。镜像发布在 `ghcr.io/yuanshi76/go-file:latest`。

## 原版特点
1. 无需配置环境，仅单个可执行文件，**直接双击即可开始使用**。
2. 自动打开浏览器，分享文件快人一步。
3. 提供**二维码**，可供移动端扫描下载文件，告别手动输入链接。
4. 支持**分享本地文件夹**。
5. 适配移动端。
6. 内置**图床**，支持直接粘贴上传图片，提供图片上传 API。
7. 内置**视频播放**页面，可在其他设备上在线观看本机视频，轻松跨设备看视频。
8. 支持**拖拽上传、拷贝上传**。
9. 允许对不同类型的用户设置文件访问权限限制。
10. 访问频率限制。
11. 支持 Token API 验证，便于与其他系统整合。
12. **支持 PicGo**，插件搜索 `gofile` 即可安装，[详见此处](https://github.com/songquanpeng/picgo-plugin-gofile)。

## 本分支相比原版的改进

### 安全加固
- **密码 bcrypt 哈希**：账户密码不再明文/弱散列存储；去除了硬编码的默认管理员密码，首次启动时若未设置 `ADMIN_PASSWORD` 则随机生成并打印到日志，提醒立即修改。
- **会话安全**：Session Cookie 加上 `HttpOnly` / `SameSite`，并可通过 `COOKIE_SECURE=true` 强制 HTTPS 下使用 `Secure`；`SESSION_SECRET` 支持持久化，避免重启后所有会话失效。
- **CSRF 防护**：对会改变状态的接口增加 CSRF 校验。
- **路径遍历加固**：统一用 `common.IsSubPath` 校验，杜绝 `..` 越权读取宿主机文件。
- **登录限流兜底**：未配置 Redis 时也有内存级登录失败限流，缓解暴力破解。
- **视频下载权限**：`/video` 资源纳入与文件一致的访问权限控制。
- **开放重定向修复**、**删除接口错误回包修复**（原先删除失败仍返回 `success:true`）、**XSS 加固**等若干修复。
- **完善的前端错误提示**：上传/删除/设置等操作在失败时会明确提示服务器返回的错误，而不是静默刷新，便于定位问题。

### 冷存储归档（OSS + WebDAV）
- 长期未访问的文件自动上传到**阿里云 OSS**（付费通道，OSS V4 签名），本地仅留占位；用户访问时再经 **WebDAV**（免费通道）自动取回，取回后在保留窗口内再次归档，节省本地与带宽成本。详见 [冷存储归档](#冷存储归档oss--webdav)。

### AI 适配
- 新增机器友好的只读发现 + 上传/下载 **AI API**（`/api/ai/*`），让 AI 代理可自主检索、下载、回传文件并读取统计。
- 针对**弱模型**（如 MiniMax-M2.5 + Hermes）做了专项优化：提供「按文件名一步下载」「先查后取」等单步接口，输入容错（可传 id 或文件名、容忍引号/空格），返回带自然语言 `summary` 提示。
- 随附标准 [OpenAPI 3.0](./docs/openapi.yaml)、[MCP](./docs/mcp-tools.json) 与 [Hermes 工具集](./docs/hermes-tools.json) 三份 Schema，可直接接入主流 AI 框架。详见 [AI 可调用 API](#ai-可调用-apiai-适配)。

### 部署友好
- **配置全面环境变量化**：所有 OSS / WebDAV / 归档参数均可通过环境变量注入，**环境变量优先于设置页的值**，且仅驻留内存、**绝不写入数据库**（密钥不落库）。
- 提供开箱即用的 [`docker-compose.yml`](./docker-compose.yml)：go-file + Redis、自定义桥接网络、Redis 健康检查 + `depends_on`，一条命令拉起。

## 快速开始

### 本地直接运行
首次启动会创建管理员账户，用户名 `admin`。**强烈建议**通过 `ADMIN_PASSWORD` 指定初始密码；若不指定，将随机生成并打印在启动日志中。登录后请到 `管理页面 → 账户管理` 修改密码。

```bash
# 指定端口、共享目录、视频目录、初始管理员密码
ADMIN_PASSWORD=your-strong-password ./go-file --port 3000 --path ./share --video ./videos
```

常用启动参数：

| 参数 | 说明 |
|------|------|
| `--port 80` | 监听端口（默认 3000） |
| `--path ./dir` | 分享指定文件夹，导航栏「文件」可见 |
| `--video ./dir` | 分享视频目录，导航栏「视频」可见 |
| `--host x.x.x.x` | 多网卡时指定对外 IP，保证二维码正确 |
| `--no-browser true` | 启动时不自动打开浏览器 |

## 环境变量一览

> 提示：环境变量优先于设置页中的同名配置；OSS/WebDAV 密钥仅驻留内存，不会写入数据库。

### 应用与存储
| 变量 | 说明 | 默认 |
|------|------|------|
| `ADMIN_PASSWORD` | 首次建库时的管理员初始密码（仅首次生效） | 随机生成 |
| `SESSION_SECRET` | 会话签名密钥 | 随机生成 |
| `SESSION_SECRET_PATH` | 会话密钥持久化文件路径 | 工作目录 |
| `COOKIE_SECURE` | HTTPS 反代时设为 `true` | `false` |
| `PORT` | 监听端口（容器内通常为 3000） | `3000` |
| `UPLOAD_PATH` | 文件上传目录 | `./upload` |
| `SQLITE_PATH` | SQLite 数据库文件路径 | `./go-file.db` |
| `SQL_DSN` | 改用 MySQL，如 `root:123456@tcp(localhost:3306)/gofile` | 空（用 SQLite） |
| `REDIS_CONN_STRING` | 启用速率限制等，如 `redis://:pass@redis:6379` | 空 |

### 冷存储归档
| 变量 | 说明 |
|------|------|
| `ARCHIVE_ENABLED` | 归档总开关，`true` / `false` |
| `ARCHIVE_AFTER_DAYS` | 多少天未访问后归档（正整数） |
| `OSS_BUCKET` | OSS Bucket 名 |
| `OSS_ENDPOINT` | OSS Endpoint，如 `https://oss-cn-beijing.aliyuncs.com` |
| `OSS_REGION` | OSS Region，可留空（从 Endpoint 自动推断） |
| `OSS_KEY_PREFIX` | 对象键前缀（可选） |
| `OSS_ACCESS_KEY_ID` | OSS AccessKey ID |
| `OSS_ACCESS_KEY_SECRET` | OSS AccessKey Secret（**密钥**） |
| `OSS_SECURITY_TOKEN` | STS 临时令牌（可选） |
| `WEBDAV_BASE_URL` | WebDAV 地址（须与 OSS 指向同一后端存储） |
| `WEBDAV_USERNAME` | WebDAV 用户名 |
| `WEBDAV_PASSWORD` | WebDAV 密码（**密钥**） |
| `WEBDAV_ROOT_PREFIX` | WebDAV 路径前缀（可选） |

## 使用 Docker Compose 部署（推荐）

仓库内的 [`docker-compose.yml`](./docker-compose.yml) 已编排好 go-file 与 Redis，并通过自定义桥接网络互联：

```bash
# 1. 复制一份并填入真实配置（切勿提交含密钥的副本）
cp docker-compose.yml docker-compose.override.yml   # 或直接编辑

# 2. 把所有「请改成...」占位值替换为你的配置
#    注意 Redis 密码在 3 处必须完全一致

# 3. 启动
docker compose up -d
```

数据持久化在宿主机的 `./data`（SQLite 与本地文件）与 `./redis-data`。仓库内的 `docker-compose.yml` 仅含占位值，真实密钥请勿提交。

### 仅运行单容器
```bash
docker run -d --restart always -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e ADMIN_PASSWORD=your-strong-password \
  -v /home/ubuntu/data/go-file:/data \
  ghcr.io/yuanshi76/go-file:latest
```

## 冷存储归档（OSS + WebDAV）

适用于「文件多、但大部分长期不访问」的场景：

1. **归档**：后台任务定期扫描，发现某文件距上次访问超过 `ARCHIVE_AFTER_DAYS` 天，便经 OSS（付费上传通道）上传到对象存储，本地仅保留元数据。
2. **取回**：用户再次访问该文件时，自动经 WebDAV（免费下载通道）取回到本地并正常提供下载。
3. **再归档**：取回后在保留窗口内若再次长期未访问，会被重新归档。

> WebDAV 必须指向与 OSS Bucket **相同的后端存储**，否则下载通道读不到上传的对象。设置页对应面板为**只读状态展示**（是否启用、配置是否就绪等），实际配置请通过环境变量注入。

## 进阶用法
- **Token API**：在个人账户页生成 Token，请求时加 HTTP 头 `Authorization: YOUR_TOKEN` 或 `Bearer YOUR_TOKEN`。
  - 例如作为 Typora 的 Image Uploader：[./script/typora.py](./script/typora.py)。
- **权限**：默认访客可上传下载，可在 `管理 → 系统设置` 中调整。
- **公网部署**：务必第一时间设置/修改管理员密码，并建议置于 HTTPS 反代之后并设 `COOKIE_SECURE=true`。

## AI 可调用 API（AI 适配）

为「让 AI 代理自主获取数据、下载与回传文件」设计的一组机器友好接口，统一挂在 `/api/ai/*`，返回 `{success, message, data}` 信封格式。

### 鉴权
- 在 `用户设置页` 生成 Token，请求时携带 HTTP 头 `Authorization: YOUR_TOKEN`（也兼容 `Bearer YOUR_TOKEN`）。
- 仅 `GET /api/ai/manifest` 为公开自描述文档，无需鉴权；其余接口均需 Token。
- 携带 `Authorization` 头的请求自动豁免 CSRF 校验，方便程序化调用。

### 接口一览
| 方法 & 路径 | 说明 |
|------|------|
| `GET /api/ai/manifest` | 自描述清单（公开），列出全部接口供 AI 自动发现 |
| `GET /api/ai/find?q=&limit=` | 按文件名/关键词查找，返回候选与自然语言 `summary` |
| `GET /api/ai/download?q=` | 按 id 或文件名**一步下载**，服务端解析最匹配项返回二进制 |
| `GET /api/ai/files?q=&page=&page_size=` | 分页列出/搜索文件 |
| `POST /api/ai/files` | 上传/回传文件（`multipart/form-data`，字段 `file` 可重复） |
| `GET /api/ai/files/{id}` | 获取单个文件元数据 |
| `GET /api/ai/files/{id}/content` | 下载文件二进制（归档至冷存储者自动取回后返回） |
| `GET /api/ai/stats` | 文件维度统计（总数、占用、下载量、类型分布等） |

### 弱模型友好
针对 MiniMax-M2.5 等能力较弱模型，刻意提供**单步、按文件名即可完成**的接口：
- `find` / `download` 的 `q` 既可是数字 id，也可是（部分）文件名，并容忍引号、空格、`7.0` 之类输入。
- 返回里的 `summary` 字段是给模型的中文提示，便于其决定下一步。
- 无需先查 id 再下载——直接 `download?q=年度报告` 即可。

### 接入 Schema
随仓库提供三份可直接使用的工具定义：
- [`docs/openapi.yaml`](./docs/openapi.yaml)：标准 OpenAPI 3.0.3 规范。
- [`docs/mcp-tools.json`](./docs/mcp-tools.json)：MCP 工具定义（list/get/download/upload/stats）。
- [`docs/hermes-tools.json`](./docs/hermes-tools.json)：为 Hermes + 弱模型调优的 4 工具精简集，含推荐 system prompt 与端点映射。

> 使用前把 Schema 中的 `base_url` / `server` 占位地址替换为你的实际部署地址，并注入 Token。

## 演示
以下展示图片来自原版，交互一致，可能未及时更新。
![index page](https://user-images.githubusercontent.com/39998050/178138784-2fc53a83-917d-4d2e-9aad-6c6c796bd9c8.png)
![file page](https://user-images.githubusercontent.com/39998050/178138792-1d9256f2-2ada-43c4-b646-28a93a919596.png)
![image page](https://user-images.githubusercontent.com/39998050/178138803-2a4da042-c29a-47c5-9e71-ebfac02cdf48.png)
![video page](https://user-images.githubusercontent.com/39998050/177032588-8946abde-a8da-45a2-a389-c16dba9cea34.png)
![setting page](https://user-images.githubusercontent.com/39998050/178138817-3f9caf95-ffc9-45fe-b2af-32c4a2e7b085.png)

## 致谢
- 原项目：[songquanpeng/go-file](https://github.com/songquanpeng/go-file)（MIT License）。
- 本分支在其基础上做安全加固与冷存储归档增强，遵循原项目许可证。
