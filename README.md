# ChargeGuard 公用充电站安全监管平台

ChargeGuard 面向公益诉讼检察官、行政监管人员、充电设施运营企业和现场巡检员，记录公用充电站的消防器材、防撞设施、警示标识和日常巡检情况，并把隐患从发现、派发、整改推进到复查销号。

## 业务流程

- 监管人员登记站点及运营责任主体。
- 巡检员提交灭火器失效、防撞隔离缺失、警示标识缺失和台账空白等隐患。
- 监管人员按期限和责任主体派发整改，运营人员提交整改证据。
- 巡检员或监管人员复查，合格隐患销号，不合格隐患退回重新整改。
- 每次状态变化写入审计记录，后台任务负责超期提醒、失败重试和永久失败记录。

状态流转为 `open -> assigned -> rectified -> verified`，复查不合格时回到 `rejected -> assigned`。所有写操作经过真实 SQLite 事务和版本控制，服务重启后可恢复站点、隐患、巡检、审计和会话数据。

## 身份与权限

内置演示账号：`prosecutor/prosecutor-demo`、`regulator/regulator-demo`、`operator/operator-demo`、`inspector/inspector-demo`。登录返回可撤销会话 Token；Token 有过期时间，退出后立即失效。检察官和监管人员可以建站、派发和复查，巡检员可以上报隐患，运营人员只能处理分配给自己的整改。

## 目录

```text
cmd/chargeguard/       HTTP 服务入口
cmd/chargeguardctl/    维护命令
internal/auth/          登录、会话、角色鉴权
internal/charging/      充电站隐患领域服务
internal/domain/        状态机和值对象
internal/service/       事务业务编排、批处理和审计
internal/storage/       SQLite repository、迁移和重启恢复
internal/httpapi/       HTTP API、请求 ID、统一错误和健康检查
internal/scheduler/     超时扫描、重试和优雅停止
internal/audit/         结构化日志与审计
migrations/             版本化 SQL 迁移
```

## 运行

需要 Go 1.26 和 `GOTOOLCHAIN=local`：

```bash
export GOTOOLCHAIN=local
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
go run ./cmd/chargeguard
```

默认监听 `:56058`，数据目录为 `./data`。可通过 `CHARGEGUARD_PORT`、`CHARGEGUARD_DATA_DIR`、`CHARGEGUARD_LOG_LEVEL` 等环境变量覆盖配置。`/healthz` 检查存活，`/readyz` 检查数据库迁移和连接。

## 公开 API

```text
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/charging/stations
GET  /api/v1/charging/stations
POST /api/v1/charging/hazards
POST /api/v1/charging/hazards/{id}/assign
POST /api/v1/charging/hazards/{id}/rectify
POST /api/v1/charging/hazards/{id}/verify
GET  /api/v1/audit
GET  /api/v1/overdues
GET  /healthz
GET  /readyz
```

## 示例

```bash
TOKEN=$(curl -s localhost:56058/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"regulator","password":"regulator-demo"}' | jq -r .id)
curl -X POST localhost:56058/api/v1/charging/stations \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"st-001","name":"人民路充电站","county":"汝南县","operator_id":"u-operator"}'
```

## 数据与迁移

生产路径使用 SQLite WAL 和真实 SQL，migration 在服务启动时幂等执行。数据库包含身份、站点、隐患、巡检、审计、事件、批次和失败重试等关联表；不使用内存 map 代替主流程持久化。
