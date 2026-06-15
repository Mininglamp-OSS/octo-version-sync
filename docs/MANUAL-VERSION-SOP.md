# 人工维护 runtime_latest_version SOP(替代 octo-version-sync)

version-sync 退役后,octo-fleet 的 `runtime_latest_version` 表改为人工维护。
本文档是发布新版本时更新该表的操作手册。

## 何时需要更新

- claude / openclaw 发布新版本(provider 升级提示 + 升级 gate 读此表)。
- octo-daemon 发布新版本(**daemon 自升级必须有 `release_meta`**:assets + checksums)。
- octo 插件(openclaw-channel-octo)发布新版本。

> codex/hermes 已退役,**不需要**维护。

## 端点

```
POST /v1/internal/runtime-latest-versions
X-Runtime-Admin-Token: $OCTO_RUNTIME_ADMIN_TOKEN
Content-Type: application/json
```

校验(fleet `upsertLatestVersionAdmin`):`component` 必须 ∈ {octo-daemon, octo,
active provider(claude/openclaw)};`latest_version` 须匹配
`^v?\d+\.\d+(\.\d+)?([-+].+)?$`;`release_meta` 可选,省略 / `null` 时 fleet
**保留旧值**(不清空)。

> ⚠️ **fleet 只校验 `release_meta` 是合法 JSON,不校验 schema / asset 覆盖 / checksum 完整性。**
> 即 `release_meta: []`、缺 assets、缺 checksums 都能写入成功 —— 错误要到 daemon 升级建任务时
> 才暴露为 `invalid release metadata` / 找不到 asset / 缺 checksum。**200 OK 不代表 daemon 能升级**,
> octo-daemon 发布必须人工对照下方结构自检。

## 场景 A:provider 版本(claude / openclaw)—— 不需要 release_meta

provider 升级时 daemon 调各自的 update 子命令(`claude update`、`openclaw update --yes
--timeout 600 --tag <target>`),不需要 url/checksum;fleet 只读 `latest_version`、建任务时
download_url/checksum 留空。只填版本号:

```json
{
  "component": "claude",
  "latest_version": "2.2.0"
}
```

(省略 `release_meta`,fleet 不动旧值。)

## 场景 B:octo-daemon 自升级 —— 必须填完整 release_meta

daemon 自升级要 fleet 提供下载 asset + checksum。从 octo-daemon-cli 的 GitHub
Release 页面取每个平台的 archive + 其 sha256(GoReleaser 生成,archive 名模板
`{{.ProjectName}}_{{.Version}}_{{.Os}}_{{.Arch}}.tar.gz`,checksum 在 `checksums.txt`)。
**`name` 必须逐字复制 Release 页面的 asset 名,不要手写猜测**:

```json
{
  "component": "octo-daemon",
  "latest_version": "0.3.0",
  "release_meta": {
    "tag": "v0.3.0",
    "assets": [
      {
        "name": "octo-daemon_0.3.0_darwin_arm64.tar.gz",
        "url": "https://github.com/Mininglamp-OSS/octo-daemon-cli/releases/download/v0.3.0/octo-daemon_0.3.0_darwin_arm64.tar.gz",
        "size": 2827327,
        "os": "darwin",
        "arch": "arm64",
        "kind": "archive"
      }
    ],
    "checksums": {
      "octo-daemon_0.3.0_darwin_arm64.tar.gz": "sha256:e87cb7..."
    }
  }
}
```

字段要求(对齐 fleet `releaseMetaJSON` / daemon 升级匹配逻辑):

- daemon 按 `os` + `arch` 找 `kind=="archive"` 的 asset;**每个目标平台都要有一条**
  (darwin/linux × arm64/amd64 等,按实际发布的平台)。
- `checksums` 的 key 必须**逐字等于**对应 asset 的 `name`,值为 `sha256:<hex>`。
- 缺 `release_meta` 或缺匹配 asset/checksum → daemon 升级报 `no release metadata available` /
  找不到 asset / 缺 checksum,升级按钮取错。

## 场景 C:octo 插件版本

```json
{
  "component": "octo",
  "latest_version": "0.7.0"
}
```

(daemon 当前通过 `npx -y create-openclaw-octo install` 执行 octo 插件升级;fleet 插件
路径只读 `latest_version`,不需要 release_meta。)

## 校验更新成功

更新后在 web Runtimes 页确认对应 component 的版本提示 / 升级按钮显示正确;
或直接查 fleet `runtime_latest_version` 表。注意场景 B 的 `200 OK` 不代表可升级,
需实际触发一次 daemon 升级或人工核对 assets/checksums 完整。

## 获取 sha256 的方法

```bash
# 下载 asset 后
shasum -a 256 octo-daemon_0.3.0_darwin_arm64.tar.gz
# 或从 GitHub Release 的 checksums.txt 取(GoReleaser 生成)
```
