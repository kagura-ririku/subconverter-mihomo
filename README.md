# subconverter-mihomo

一个基于 `mihomo` 的轻量订阅转换服务。

它不依赖第三方在线 `subconverter` 服务，而是直接调用本地 `mihomo` 拉取和解析上游订阅，再由当前服务完成节点筛选、重命名、订阅命名和远程配置编译，最后下发给 Clash/Mihomo 客户端。

预构建镜像：

```text
ghcr.io/kagura-ririku/subconverter-mihomo:latest
```

## 功能

- 使用 `mihomo` 实时拉取和解析上游订阅
- 支持多个订阅组，每个订阅组使用自己的 UUID 鉴权
- 每个订阅组可配置多个上游订阅
- 每个订阅组可配置固定远程配置
- 支持订阅级和上游级节点筛选
- 自动重命名节点并添加地区旗帜
- 可按固定顺序排序节点：`港澳台日韩新美`
- 保留上游返回的 `subscription-userinfo`
- 支持按路径区分订阅格式，例如 `https://your-domain/<uuid>/clash`
- 不支持任何查询参数

## 架构

- `subconverter`：Go 服务，对外提供订阅地址
- `mihomo`：sidecar，负责拉取和解析上游订阅

两个容器共享同一个运行时 volume，用来交换 `mihomo` 配置和 provider 缓存。这部分是运行时数据，不是长期业务数据。

## 目录

```text
cmd/subconverter/main.go      程序入口
internal/app                  HTTP 请求流程
internal/nodes                节点筛选、去重、重命名、排序
internal/remoteconfig         远程配置解析和编译
internal/mihomo               mihomo runtime/controller 交互
config/subscriptions.json     订阅配置文件
docker-compose.yml            部署配置
```

## 部署

有两种常见方式：

1. 直接 `git clone` 整个仓库
2. 只下载部署需要的最小文件

如果你不打算 clone 整个仓库，最少需要这两个文件：

- [docker-compose.yml](docker-compose.yml)
- [config/subscriptions.json](config/subscriptions.json)

目录结构需要是这样：

```text
your-workdir/
├── docker-compose.yml
└── config/
    └── subscriptions.json
```

例如：

```bash
mkdir -p subconverter-mihomo/config
cd subconverter-mihomo
```

然后把 `docker-compose.yml` 放在当前目录，把 `subscriptions.json` 放到 `config/` 目录下。

## 快速开始

1. 准备好 [docker-compose.yml](docker-compose.yml) 和 [config/subscriptions.json](config/subscriptions.json)。

2. 修改 [config/subscriptions.json](config/subscriptions.json)，填入你自己的 UUID、上游订阅和远程配置 URL。

3. `uuid` 可以自己生成，也可以直接上网搜索 `uuid generator`，生成一个随机 UUID v4 后填进去。

4. 如果需要改对外端口，修改 [docker-compose.yml](docker-compose.yml) 里的 `127.0.0.1:7000:8080`，只改中间的 `7000` 即可。

5. 在 `docker-compose.yml` 所在目录启动：

```bash
docker compose pull
docker compose up -d
```

6. 访问订阅：

```text
https://your-domain/<uuid>/clash
```

当前默认支持 `clash`，所以旧的 `https://your-domain/<uuid>` 也仍然兼容。

## 配置文件

[config/subscriptions.json](config/subscriptions.json) 是一个数组，每一项代表一个订阅组。

字段说明：

- `uuid`：客户端访问该订阅时使用的 UUID
- `name`：返回给客户端的订阅文件名
- `remote_config`：固定远程配置 URL
- `sort_nodes_by_region`：是否按固定地区顺序排序节点
- `include_regex` / `exclude_regex`：订阅级筛选规则
- `upstreams`：上游订阅列表

上游字段说明：

- `url`：上游订阅地址
- `user_agent`：请求上游时使用的 UA，留空时默认 `MetaCubeX/mihomo`
- `headers`：请求上游时附带的请求头，通常留空；只有机场要求额外鉴权头时才需要填写
- `include_regex` / `exclude_regex`：仅对当前上游生效的筛选规则

## 示例

[config/subscriptions.json](config/subscriptions.json)

## 默认部署

默认情况下不需要改容器环境变量。

当前 `docker-compose.yml` 只需要关心两件事：

- `./config:/app/config:ro`：把本地订阅配置挂进容器
- `127.0.0.1:7000:8080`：本机只监听 `127.0.0.1:7000`

如果你前面挂了 Nginx 反代，通常只需要改这一处端口映射。

如果后面确实需要覆盖默认行为，程序仍然支持这些可选环境变量：

- `SUBCONVERTER_ALLOWED_HOSTS`
- `SUBCONVERTER_REQUEST_TIMEOUT_SECONDS`
- `SUBCONVERTER_CONTROLLER_STARTUP_TIMEOUT_SECONDS`
- `SUBCONVERTER_SUBSCRIPTIONS_FILE`

## 接口

- `GET /healthz`：存活检查
- `GET /readyz`：`mihomo` 就绪检查
- `GET /<uuid>/clash`：获取 Clash 订阅
- `GET /<uuid>`：兼容旧路径，当前等同于 `GET /<uuid>/clash`

说明：

- 请求里带任何查询参数都会返回 `400`
- 当前只有 `clash` 格式可用，其他格式路径会返回 `400`
- 如果配置了 `SUBCONVERTER_ALLOWED_HOSTS`，不在白名单内的 Host 会返回 `403`
- 找不到对应 UUID 也会返回 `403`

## 支持的远程配置能力

当前实现的是面向 Clash/Mihomo 的常用子集，主要支持：

- `custom_proxy_group`
- `ruleset`
- `clash_rule_base`
- `rename`
- `include_remarks`
- `exclude_remarks`
- `!!import:...`

这里只支持远程 URL，不支持本地规则文件路径。
