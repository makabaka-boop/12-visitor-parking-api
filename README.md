# 12-visitor-parking-api

> **题目正文（完整）**
>
> 12｜【纯后端API】社区访客停车授权接口
>
> 设计一个服务于住宅社区的访客停车后台接口集，处理住户报备、车辆入场授权和离场核销，供物业值班人员、门岗设备与内部程序使用。核心业务以 Go 实现，长期数据保存到 PostgreSQL，服务固定监听 8117 端口。建立住户、车辆、停车区域、访客授权、进出记录和操作日志模型，提供创建、详情、更新、软归档及分页列表接口；授权列表可按楼栋、车牌、有效日期、状态和区域筛选。新增授权时验证住户已启用、车牌格式正确、开始时间不早于当前时间、结束时间晚于开始时间，单次授权不超过七天，同一车牌不得存在重叠的有效授权。车辆入场时校验授权时段和区域剩余车位，并在事务内生成记录、占用名额；重复请求返回冲突。离场时记录时间、操作人和备注并释放名额，已离场记录不得再次核销。允许撤销尚未使用的授权，已入场授权只能先离场。提供当前在场车辆、今日预计到访、即将过期授权和区域占用率统计。更新接口基于更新时间检查并发冲突，所有状态迁移验证前置状态。历史进出记录永久保留，其他数据仅软删除，关键变更写审计日志。统一响应、错误码和参数校验明细，提供健康检查、结构化日志、迁移脚本及必要接口测试。代码总量不超过 3000 行。交付独立多阶段 Dockerfile，使用官方多架构镜像按目标平台编译，支持 linux/arm64 与 linux/amd64；通过 EXPOSE 暴露 8117，数据库连接由环境变量注入，README 给出典型流程、curl、双平台构建和启动命令。

社区访客停车授权接口：Go + PostgreSQL 后端，监听 8117。住户报备 → 授权创建 → 车辆入场 → 离场核销 的全链路 API。

## 目录结构

```
.
├── cmd/server/main.go          # 入口，监听 8117，加载配置与迁移
├── internal/
│   ├── config/                 # 环境变量配置
│   ├── model/                  # 领域模型与状态常量
│   ├── plate/                  # 车牌格式校验
│   ├── store/                  # Store 接口 + 内存实现 + PostgreSQL 实现
│   ├── service/                # 业务逻辑、校验、状态迁移、审计
│   ├── httpd/                  # HTTP 路由、统一响应、结构化日志、健康检查
│   └── migrations/             # 内嵌 SQL 迁移脚本
├── Dockerfile                  # 生产多阶段镜像（官方多架构，容器内按平台编译）
├── benzhi.Dockerfile           # 评测镜像（保留完整 Go 工具链）
├── build_benzhi_docker.sh      # 评测构建脚本
├── BENZHI_README.md            # 评测说明
└── README.md                   # 本文件
```

## 设计要点

- **可替换存储**：业务逻辑仅依赖 `store.Store` 接口。生产用 PostgreSQL；测试用 `store.NewMemory()` 内存实现，**测试不依赖任何外部数据库**。
- **事务**：入场/离场/撤销/创建授权在事务内完成（PostgreSQL 用 `SERIALIZABLE` 事务 + `FOR UPDATE`；内存实现用全局互斥锁），避免容量与重叠校验的 TOCTOU。
- **软删除**：除进出记录永久保留外，其余实体通过 `archived_at` 软归档。
- **审计**：关键变更写 `audit_logs`。
- **并发检查**：更新接口要求客户端回传 `updated_at`，服务端校验版本一致后再更新。

## 状态机

```
授权状态：
  pending ──entry──▶ active ──exit──▶ completed   （正常流）
  pending ──revoke──▶ cancelled                    （物业撤销，未入场）
  pending ──(超过 end_time)──▶ expired             （动态判定）
  active 只能 exit，不能 revoke；completed/cancelled 为终态。
```

## 快速开始

### 本地运行（内存存储，无需数据库）

```bash
export STORAGE_DRIVER=memory
export HTTP_ADDR=:8117
go run ./cmd/server
```

### 连接 PostgreSQL

```bash
export STORAGE_DRIVER=postgres
export DATABASE_URL="postgres://user:pass@localhost:5432/visitor_parking?sslmode=disable"
export HTTP_ADDR=:8117
go run ./cmd/server            # 启动时自动执行迁移
```

### 编译与测试

```bash
go build ./...                 # 编译
go test ./...                  # 全部测试（内存存储，无外部依赖）
go vet ./...                   # 静态检查
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8117` | 监听地址 |
| `DATABASE_URL` | （空） | PostgreSQL DSN，`STORAGE_DRIVER=postgres` 时必填 |
| `STORAGE_DRIVER` | `postgres` | `postgres` 或 `memory` |
| `LOG_LEVEL` | `info` | 日志级别 |

## 典型流程：报备至离场

1. **创建停车区域**（物业）
   ```bash
   curl -s -X POST localhost:8117/api/v1/areas \
     -H 'Content-Type: application/json' \
     -d '{"name":"A区地下","code":"A","capacity":50}'
   # → data.id = area_...
   ```
2. **创建住户**
   ```bash
   curl -s -X POST localhost:8117/api/v1/residents \
     -H 'Content-Type: application/json' \
     -d '{"name":"张三","phone":"13800000000","building":"1栋","unit":"2","room":"301"}'
   # → data.id = res_...
   ```
3. **住户报备访客授权**（开始/结束时间为 RFC3339）
   ```bash
   curl -s -X POST localhost:8117/api/v1/authorizations \
     -H 'Content-Type: application/json' \
     -d '{
       "resident_id":"res_...",
       "parking_area_id":"area_...",
       "plate":"京A12345",
       "start_time":"2026-08-16T10:00:00Z",
       "end_time":"2026-08-16T18:00:00Z",
       "purpose":"拜访","created_by":"staff1"
     }'
   # → data.id = auth_..., status=pending
   ```
4. **车辆入场**（门岗设备）
   ```bash
   curl -s -X POST localhost:8117/api/v1/authorizations/auth_.../entry
   # → data.status=entered, 授权变 active，占用一个名额
   # 重复请求 → 409 冲突
   ```
5. **车辆离场**（门岗/值班人员核销）
   ```bash
   curl -s -X POST localhost:8117/api/v1/authorizations/auth_.../exit \
     -H 'Content-Type: application/json' \
     -d '{"operator":"guard1","note":"正常离场"}'
   # → data.status=exited, 授权变 completed, 释放名额
   # 已离场再次核销 → 409
   ```
6. **物业撤销未使用授权**（仅 pending 可撤销）
   ```bash
   curl -s -X POST localhost:8117/api/v1/authorizations/auth_.../revoke \
     -H 'Content-Type: application/json' \
     -d '{"operator":"mgr","reason":"住户取消"}'
   # 已入场的授权撤销 → 409（需先离场）
   ```

## 接口一览

所有响应统一为 `{code, success, data?, error?}`，错误体含 `message` 与可选 `fields[]` 校验明细。`GET` 列表支持 `limit`/`offset` 分页，返回 `{items,total,page,size}`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| POST/GET | `/api/v1/residents` | 创建 / 分页列表 |
| GET/PUT/DELETE | `/api/v1/residents/{id}` | 详情 / 更新(需 updated_at) / 软归档 |
| POST/GET | `/api/v1/vehicles` | 创建 / 分页列表 |
| GET/PUT/DELETE | `/api/v1/vehicles/{id}` | 详情 / 更新 / 软归档 |
| POST/GET | `/api/v1/areas` | 创建 / 分页列表 |
| GET/PUT/DELETE | `/api/v1/areas/{id}` | 详情 / 更新 / 软归档 |
| POST/GET | `/api/v1/authorizations` | 创建 / 筛选列表 |
| GET/PUT/DELETE | `/api/v1/authorizations/{id}` | 详情 / 更新 / 软归档 |
| POST | `/api/v1/authorizations/{id}/revoke` | 撤销（仅 pending） |
| POST | `/api/v1/authorizations/{id}/entry` | 车辆入场 |
| POST | `/api/v1/authorizations/{id}/exit` | 车辆离场 |
| GET | `/api/v1/records` | 进出记录（area_id/status/plate 筛选） |
| GET | `/api/v1/stats/current-vehicles` | 当前在场车辆（可加 area_id） |
| GET | `/api/v1/stats/today-arrivals` | 今日预计到访 |
| GET | `/api/v1/stats/expiring-soon` | 即将过期授权（6h 内） |
| GET | `/api/v1/stats/occupancy` | 各区域占用率 |
| GET | `/api/v1/audit-logs` | 审计日志（entity_type 筛选） |

### 授权列表筛选（query string）

`building`、`plate`、`area_id`、`status`、`valid_on`(RFC3339)、`today=1`、`expiring_soon=1`、`limit`、`offset`，结果按 `start_time` 降序。

```bash
curl -s 'localhost:8117/api/v1/authorizations?building=1栋&status=pending&limit=10'
curl -s 'localhost:8117/api/v1/stats/occupancy'
```

## 创建授权校验规则

- 住户存在且状态为 `active`（已停用 → 400）
- 车牌格式正确（省汉字+字母+5~6 位）
- `start_time` 不早于当前时间
- `end_time` 晚于 `start_time`
- 单次时长 ≤ 7 天
- 同一车牌在重叠时间内只能存在一条有效授权（pending/active），冲突 → 409

## Docker

### 生产镜像（多阶段，官方多架构）

```bash
# 单平台构建
docker build --platform linux/arm64 -t visitor-parking:arm64 .
docker build --platform linux/amd64 -t visitor-parking:amd64 .

# 多平台一并构建并推送（需 buildx）
docker buildx build --platform linux/amd64,linux/arm64 -t visitor-parking:latest --push .
```

容器内按 `TARGETOS/TARGETARCH` 编译（`CGO_ENABLED=0`，lib/pq 为纯 Go）。`Dockerfile` 通过 `EXPOSE 8117` 暴露端口，数据库连接由环境变量注入。

### 运行（宿主机端口映射）

```bash
docker run --rm -p 8117:8117 \
  -e DATABASE_URL="postgres://user:pass@host.docker.internal:5432/visitor_parking?sslmode=disable" \
  -e STORAGE_DRIVER=postgres \
  visitor-parking:latest
# 健康检查
curl localhost:8117/healthz
```

> 无数据库时可用内存模式：`-e STORAGE_DRIVER=memory`（数据不持久）。

### 评测镜像

见 `BENZHI_README.md` 与 `build_benzhi_docker.sh`。

## 测试

```bash
go test ./... -v        # 服务层 + HTTP 层，内存存储，无外部依赖
go test -race ./...     # 竞态检测
```

覆盖：创建授权全部校验规则、重叠冲突、入场/离场状态迁移、容量限制、重复入场/离场冲突、撤销前置状态、更新并发冲突、统计接口、软删除、统一错误响应与校验明细。
