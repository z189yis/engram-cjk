# Fork Release Engineering

本文件记录 ZhiYuan fork 的发布工程约定（方案 §7 Phase 5 交付）。

## 版本方案

Fork 使用专属预发布后缀，绝不修改上游版本号发布：

```text
v1.20.0-zhiyuan.1   # 首次 fork 发布（基于上游 v1.20.0）
v1.20.0-zhiyuan.2   # 同基线的修复
v1.21.0-zhiyuan.1   # 升级到新上游基线后
```

## 发布流程

1. 合并 CJK 功能 PR 到 `main`
2. 基于 `main` 打标签：`git tag v1.20.0-zhiyuan.1`
3. 推送标签触发 [fork-release.yml](../.github/workflows/fork-release.yml)
4. 工作流使用 [.goreleaser.fork.yaml](../.goreleaser.fork.yaml) 构建 6 平台资产
5. 自动校验必需资产清单 + checksums.txt

## 必需资产

| 平台 | 资产 | 校验 |
|---|---|---|
| macOS x64 | `engram_{version}_darwin_amd64.tar.gz` | 必需 |
| macOS arm64 | `engram_{version}_darwin_arm64.tar.gz` | 必需 |
| Windows x64 | `engram_{version}_windows_amd64.zip` | 必需 |
| Windows arm64 | `engram_{version}_windows_arm64.zip` | 必需 |
| Linux x64 | `engram_{version}_linux_amd64.tar.gz` | 必需 |
| Linux arm64 | `engram_{version}_linux_arm64.tar.gz` | 必需 |

所有资产附带 `checksums.txt`（SHA-256）与 SBOM（cyclonedx）。

## 与上游发布的关系

- Fork 发布使用独立工作流 `fork-release.yml`，不触碰上游 `release.yml`
- 不含 Homebrew tap 发布（避免污染上游 tap）
- 上游升级时先 rebase 补丁栈，再打新 fork 标签

## 验证清单

- [ ] 6 平台资产齐全
- [ ] `checksums.txt` 存在且与资产匹配
- [ ] 每个资产 `engram --version` 报告 fork 版本
- [ ] `/health`、会话创建、观察创建、CJK `/search` 冒烟通过
- [ ] 标签不可被工作流覆盖（GitHub 不可变标签默认行为）
