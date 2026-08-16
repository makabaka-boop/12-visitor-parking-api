# BENZHI_README — 评测说明

## 项目简介

社区访客停车授权接口（纯后端 Go API）。处理住户报备、车辆入场授权与离场核销，供物业值班人员、门岗设备与社区内部程序使用。监听 **8117** 端口，长期数据存 PostgreSQL，业务逻辑通过 `store.Store` 接口抽象，测试使用内存存储、不依赖外部数据库。

模型：住户 / 车辆 / 停车区域 / 访客授权 / 进出记录 / 操作日志，均提供创建、详情、更新、软归档、分页列表接口。

## 标准命令

```bash
go build ./...          # 编译全部包
go vet ./...            # 静态检查
go test ./...           # 运行全部测试（内存存储，无外部依赖）
go test -race ./...     # 带竞态检测
go run ./cmd/server     # 启动服务（默认 :8117）
```

## 运行环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8117` | 监听地址 |
| `STORAGE_DRIVER` | `postgres` | `postgres` 或 `memory` |
| `DATABASE_URL` | （空） | PostgreSQL DSN，`postgres` 模式必填 |
| `LOG_LEVEL` | `info` | 日志级别 |

无数据库时可 `STORAGE_DRIVER=memory go run ./cmd/server` 直接启动验证接口。

## 评测镜像构建

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh visitor-parking               # 默认 linux/amd64
./build_benzhi_docker.sh visitor-parking linux/arm64   # arm64
docker run -it visitor-parking:latest                  # 进入容器
```

容器内（基于官方 `golang:1.22`，保留完整 Go 工具链）：

```bash
go build ./...     # 应编译通过
go test ./...      # 应测试通过
go version
```

## 多架构说明

- `benzhi.Dockerfile` 基于官方多架构镜像 `golang:1.22`，未写死 CPU 架构、未 COPY 本机预编译二进制，`linux/arm64` 与 `linux/amd64` 均可构建运行。
- 生产多阶段 `Dockerfile` 用 `golang:1.22-alpine` 构建、`alpine:3.20` 运行，`CGO_ENABLED=0` 按 `TARGETOS/TARGETARCH` 在容器内编译。

## 关键文件

- `cmd/server/main.go` — 入口，加载配置、迁移、启动 HTTP 服务
- `internal/store/store.go` — 存储接口
- `internal/store/memory*.go` — 内存实现（测试用）
- `internal/store/postgres.go` — PostgreSQL 实现
- `internal/service/service.go` — 业务逻辑与校验
- `internal/httpd/` — HTTP 路由、统一响应、结构化日志、健康检查
- `internal/migrations/001_init.sql` — 建表脚本（启动时自动执行）
- `Dockerfile` — 生产多阶段镜像
