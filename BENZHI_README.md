# BENZHI_README

## 项目说明

- 项目：11DingKing/chargeguard-bug-29
- 项目用途：ChargeGuard 面向公益诉讼检察官、行政监管人员、充电设施运营企业和现场巡检员，记录公用充电站的消防器材、防撞设施、警示标识和日常巡检情况，并把隐患从发现、派发、整改推进到复查销号。
- Go 工具链：`golang:1.26`
- 前端工具链：无

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/chargeguard
cd '/app' && GOTOOLCHAIN=local go run ./cmd/chargeguardctl

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-53-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-53-arm64 linux/arm64
docker run -it benzhi-task-53-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-53-arm64:latest
```

## 题目验证命令

1. 预期退出码 0：`go test ./internal/httpapi -run TestTaskBehavior -count=1`
2. 预期退出码 0：`go test -buildvcs=false -count=1 ./...`
3. 预期退出码 0：`GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./...`

## Bug 复现

Bug 现象、触发步骤和完整错误信息见 `BUG_REPRO.md`。
