# Bucket Storage (bucketd)

企业级文件存储桶服务。上传任意文件后生成公开访问 URL，在浏览器中打开链接可直接查看图片/视频/音频/PDF，或下载其他类型文件。

## 功能特性

- **文件上传**：支持所有文件类型，最大 5GB
- **大文件分片上传**：3 步流程（初始化 → 分片 → 合并），支持断点续传
- **公开访问链接**：上传后生成 URL，浏览器直接打开
- **智能展示**：图片/视频/音频/PDF/文本在浏览器内联显示，其他类型触发下载
- **视频进度条**：支持 HTTP Range 请求，视频可拖动进度条
- **文件去重**：基于 SHA-256 内容寻址，相同文件只存一份
- **引用计数删除**：多次上传同一文件，删除一次不影响其他引用
- **Web 管理界面**：浏览器中上传、查看、编辑、删除文件
- **Base URL 自定义**：页面可设置外部访问域名，生成的链接自动同步
- **API Key 认证**：管理接口需要认证，文件访问链接公开无需认证
- **CORS 支持**：跨域请求支持，方便前端集成
- **缓存优化**：ETag + Cache-Control，浏览器强缓存减少重复请求

## 快速开始

### 前置条件

- Go 1.23+ （仅编译时需要，运行不需要）

### 编译运行

```bash
# 克隆仓库
git clone https://github.com/nbxiaominb-maker/cunchutong.git
cd cunchutong

# 编译
go build -o bucketd ./cmd/bucketd/

# 启动（默认读取 configs/bucketd.yaml）
./bucketd
```

启动后访问：

| 地址 | 说明 |
|------|------|
| `http://localhost:8080` | Web 管理界面 |
| `http://localhost:8080/healthz` | 健康检查 |

### 命令行参数

```
bucketd [选项]

选项:
  --config string     配置文件路径（默认 configs/bucketd.yaml）
  --host string       绑定地址（覆盖配置文件）
  --port int          绑定端口（覆盖配置文件）
  --data-dir string   数据存储目录（覆盖配置文件）
  --log-level string  日志级别: debug, info, warn, error
  --version           显示版本号
```

### 环境变量

所有配置项均可通过环境变量覆盖：

| 环境变量 | 说明 |
|----------|------|
| `BUCKETD_SERVER_HOST` | 绑定地址 |
| `BUCKETD_SERVER_PORT` | 绑定端口 |
| `BUCKETD_SERVER_MAX_UPLOAD_SIZE` | 最大上传大小（字节） |
| `BUCKETD_STORAGE_DATA_DIR` | 数据存储目录 |
| `BUCKETD_DATABASE_PATH` | 数据库文件路径 |

## 配置说明

配置文件路径：`configs/bucketd.yaml`

```yaml
server:
  host: "0.0.0.0"          # 绑定地址，0.0.0.0 表示所有网卡
  port: 8080                # 监听端口
  external_url: ""          # 外部访问 URL，用于生成公开链接
  read_timeout: 30s         # 读取超时
  write_timeout: 300s       # 写入超时（大文件上传需要较长时间）
  max_upload_size: 5368709120  # 最大上传 5GB

storage:
  data_dir: "./data"           # 数据根目录
  tmp_dir: "./data/files/tmp"  # 临时文件目录
  thumbnail_dir: "./data/thumbnails"  # 缩略图缓存目录

database:
  path: "./data/metadata.db"   # SQLite 数据库路径

security:
  api_keys:                     # API Key 列表
    - name: "admin"             # 名称（标识用）
      key: "bucketd_admin_key_change_me"  # Key 明文（存储时自动哈希）
      permissions: ["read", "write", "delete", "admin"]  # 权限
  allowed_types: []             # 允许的 MIME 类型，空=允许所有
  blocked_types: []             # 禁止的 MIME 类型

thumbnails:
  enabled: true                 # 启用缩略图
  default_size: 200             # 默认缩略图尺寸
  max_size: 800                 # 最大缩略图尺寸

cors:
  allowed_origins: ["*"]        # 允许的跨域来源
  max_age: 86400                # 预检请求缓存时间

logging:
  level: "info"                 # 日志级别
  format: "json"                # 日志格式
```

## Web 管理界面

浏览器打开 `http://localhost:8080` 进入管理界面。

### 使用步骤

1. **输入 API Key**：页面右上角输入 API Key（默认：`bucketd_admin_key_change_me`）
2. **设置 Base URL**：在 Base URL 输入框填入外部访问地址（如 `http://100sq08np5472.vicp.fun`），生成的文件链接会自动使用该地址
3. **上传文件**：切换到 Upload 标签，拖拽或点击上传，可选 Bucket/Tags/公开性
4. **管理文件**：切换到 Files 标签，支持搜索、筛选、编辑（重命名/改标签/改 Bucket）、删除

## API 文档

所有管理接口需要 `Authorization: Bearer <api-key>` 请求头。文件访问接口无需认证。

### 上传文件

```
POST /api/v1/files
Content-Type: multipart/form-data
Authorization: Bearer <api-key>

字段:
  file       (必填) 文件
  bucket     (可选) 桶名，默认 "default"
  tags       (可选) 逗号分隔的标签，如 "tag1,tag2"
  is_public  (可选) "true" 或 "false"，默认 true

响应 201:
{
  "id":           "01KTQYRVPBQFXA0D1MNKS9H4P8",
  "url":          "http://100sq08np5472.vicp.fun/f/7fcda6a8...",
  "filename":     "photo.jpg",
  "size":         2048576,
  "mime":         "image/jpeg",
  "sha256":       "7fcda6a891ac7e7609640f0a765b8e5d...",
  "bucket":       "default",
  "tags":         ["vacation"],
  "is_public":    true,
  "deduplicated": false,
  "created_at":   "2026-06-10T05:06:08Z"
}
```

### 大文件分片上传

**第 1 步：初始化**

```
POST /api/v1/multipart/init
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "filename": "video.mp4",
  "size":     5368709120,
  "mime":     "video/mp4",
  "bucket":   "videos"
}

响应 200:
{
  "upload_id":    "01KTQ...",
  "chunk_size":   8388608,
  "total_chunks": 640
}
```

**第 2 步：上传分片（可并行）**

```
PUT /api/v1/multipart/{upload_id}/chunks/{chunk_number}
Authorization: Bearer <api-key>
Content-Type: application/octet-stream

<请求体为原始字节>

响应 200:
{
  "chunk_number": 1,
  "sha256":       "chunk_hash...",
  "received":     true,
  "temp_path":    "/path/to/temp/chunk"
}
```

**第 3 步：合并完成**

```
POST /api/v1/multipart/{upload_id}/complete
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "chunks": [
    {"number": 1, "sha256": "hash1", "temp_path": "/path/to/chunk1"},
    {"number": 2, "sha256": "hash2", "temp_path": "/path/to/chunk2"}
  ]
}

响应 201: （同单文件上传响应格式）
```

### 获取文件列表

```
GET /api/v1/files?bucket=default&page=1&per_page=50&sort=created_at:desc
Authorization: Bearer <api-key>

响应 200:
{
  "files":    [...],
  "total":    142,
  "page":     1,
  "per_page": 50
}
```

### 获取文件详情

```
GET /api/v1/files/{id}
Authorization: Bearer <api-key>

响应 200: （同上传响应格式）
```

### 更新文件

```
PUT /api/v1/files/{id}
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "filename":  "new-name.txt",
  "bucket":    "new-bucket",
  "tags":      ["tag1", "tag2"],
  "is_public": true
}

响应 200: （同上传响应格式，含更新后的数据）
```

### 删除文件

```
DELETE /api/v1/files/{id}
Authorization: Bearer <api-key>

响应 200:
{
  "deleted":          true,
  "id":               "01KTQ...",
  "physical_deleted": true
}
```

> `physical_deleted` 表示物理文件是否已删除（引用计数归零时才删物理文件）。

### 访问文件（公开，无需认证）

```
GET /f/{sha256_hash}
```

浏览器行为：
- **图片/视频/音频/PDF/文本**：浏览器内联显示（`Content-Disposition: inline`）
- **其他类型**：触发下载（`Content-Disposition: attachment`）

响应头包含：
- `ETag`：SHA-256 哈希值
- `Cache-Control: public, max-age=31536000, immutable`：强缓存一年
- `Accept-Ranges: bytes`：支持 Range 请求（视频拖动进度条）

### 健康检查

```
GET /healthz

响应 200:
{
  "status":         "ok",
  "version":        "1.0.0",
  "uptime_seconds": 86400,
  "storage_bytes":  107374182400,
  "file_count":     5230
}
```

## 部署

### 方式 1：直接运行

```bash
# 编译
go build -ldflags="-s -w" -o bucketd ./cmd/bucketd/

# 运行
./bucketd --config configs/bucketd.yaml
```

### 方式 2：Docker

```bash
# 构建并启动
cd deploy
docker compose up -d

# 查看日志
docker compose logs -f
```

### 方式 3：systemd 服务

```bash
# 复制文件
sudo cp bucketd /usr/local/bin/
sudo cp configs/bucketd.yaml /etc/bucketd/config.yaml
sudo cp deploy/bucketd.service /etc/systemd/system/

# 创建用户
sudo useradd -r -s /bin/false bucketd
sudo mkdir -p /var/lib/bucketd
sudo chown bucketd:bucketd /var/lib/bucketd

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable --now bucketd

# 查看状态
sudo systemctl status bucketd
```

## 项目结构

```
├── cmd/bucketd/main.go               # 程序入口
├── internal/
│   ├── config/config.go              # 配置加载
│   ├── store/store.go                # 磁盘存储（哈希分片）
│   ├── meta/
│   │   ├── meta.go                   # SQLite 元数据管理
│   │   └── migrations/001_init.sql   # 数据库 Schema
│   └── handler/
│       ├── upload.go                 # 上传/分片上传
│       ├── download.go               # 文件下载/在线查看
│       ├── update.go                 # 文件更新
│       ├── delete.go                 # 文件删除
│       ├── list.go                   # 文件列表
│       ├── health.go                 # 健康检查
│       ├── middleware.go             # 认证/CORS/日志中间件
│       ├── webui.go                  # Web 界面
│       └── static/index.html         # 前端页面
├── configs/bucketd.yaml              # 配置文件
├── deploy/
│   ├── Dockerfile                    # Docker 构建
│   ├── docker-compose.yaml           # Docker Compose
│   └── bucketd.service               # systemd 服务
├── go.mod
└── go.sum
```

## 存储原理

文件采用 **内容寻址存储**：

1. 上传文件写入临时目录
2. 流式计算 SHA-256 哈希
3. 哈希前两位/次两位作为两级目录：`data/files/ab/c1/abc1a1b2c3d4...`
4. 原子重命名到最终路径
5. 如果哈希已存在，删除临时文件，增加引用计数（**自动去重**）

删除时引用计数递减，归零才删除物理文件。

## 技术栈

| 组件 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.23 | 单二进制部署，无运行时依赖 |
| 路由 | Go 1.22+ ServeMux | 标准库，无第三方路由依赖 |
| 元数据 | SQLite (modernc.org/sqlite) | 纯 Go 实现，无需 CGO |
| 日志 | log/slog | Go 标准库结构化日志 |
| ID | ULID (oklog/ulid) | 时间排序，URL 安全 |
| 配置 | gopkg.in/yaml.v3 | YAML 配置文件 |

## 许可证

MIT License
