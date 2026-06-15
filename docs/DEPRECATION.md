# octo-version-sync 退役说明(2026-06)

## 背景

octo-version-sync 是「上游版本聚合器」:每隔几分钟轮询 GitHub Releases / npm,
把规范化的 `version.json`(含各 component 的 `latest_version` + `release_meta`)
写到对象存储(COS),供 octo-fleet 拉取进 `runtime_latest_version` 表。

2026-06 的 runtime 线调整后:

- octo-fleet 的 COS 同步器 `modules/runtime/version_sync.go` **已删除**。
- `runtime_latest_version` 表改为**人工维护**(见 [`MANUAL-VERSION-SOP.md`](MANUAL-VERSION-SOP.md))。
- 因此 version-sync 已无下游消费者,停止部署。

## 最后一次产物来源

退役时 version-sync 产出的 `version.json` 来源(`components.json`):

| component | source |
|---|---|
| octo-daemon | github:Mininglamp-OSS/octo-daemon-cli |
| claude | github:anthropics/claude-code |
| codex | github:openai/codex |
| hermes | github:NousResearch/hermes-agent |
| openclaw | npm:openclaw |
| octo | github:Mininglamp-OSS/openclaw-channel-octo |

## ⚠️ components.json 与 DefaultComponents 的差异(人工接管须知)

代码里有两份 component 列表,**内容不一致**,接管时勿混淆:

| component | components.json(生产用) | DefaultComponents(model.go fallback) |
|---|---|---|
| octo-daemon | ✓ | ✓ |
| octo-version-sync | ✗ | ✓(fallback 历史默认项) |
| claude | ✓ | ✓ |
| codex | ✓ | ✗ |
| hermes | ✓ | ✓ |
| openclaw | ✓ | ✓ |
| octo | ✓ | ✓ |

- 生产实际跑的是 `components.json`(`--components` 参数指向它);`DefaultComponents`
  (`internal/model.go`)仅在未提供该文件时兜底。
- **codex/hermes 注意**:runtime 线本期已从 fleet/daemon/web 全栈移除 codex/hermes
  (只放行 claude+openclaw)。version-sync 的 components.json 仍含 codex/hermes 是
  历史产物记录;人工维护 `runtime_latest_version` 时**只需维护 active provider
  (claude/openclaw)+ octo-daemon + octo(插件)**,不必再维护 codex/hermes。

## 部署下线清单(运维执行)

version-sync 走独立 GitLab deploy-files 分支 + ArgoCD,**不在 octo-deployment kustomize 内**。下线需:

- [ ] **先禁用 CI/CD 自动部署**(否则后续 main/tag pipeline 会重建以下资源):
      - 禁用 GitLab Pull Mirror / CI deploy pipeline(`.gitlab-ci.yml` 的 `sync_code` + `argocd_sync` stage 在 main/develop/tags 触发)
      - 冻结 / 移除部署变量:`CODE_TOKEN`、`ARGOCD_TOKEN` 等
      - 确认后续 main/tag 推送不再写 deploy-files 分支
- [ ] 停止 / 删除 ArgoCD app:`octo-version-sync-dev`、`octo-version-sync-prod`
- [ ] 删除 deploy-files 仓库分支:`octo-version-sync-dev`、`octo-version-sync-prod`
- [ ] 删除 K8s namespace 下的 deployment + service(`octo-version-sync-*`)
- [ ] 回收 secret:`octo-version-sync-secrets-*`(COS URL/ID/Key、GitHub token、trigger token)
- [ ] (可选)归档 TCR 镜像 `.../dmwork/octo-version-sync`
- [ ] COS 上的旧 `version-sync/version.json` 可保留作历史,fleet 已不再读取。

## 回滚

若需临时恢复自动同步:重新部署 version-sync + 在 fleet 恢复 `version_sync.go`
(从该改动之前的 git 历史取回)。但优先用人工 SOP,不建议回滚。
