# DORA Metrics Calculator

GitHub API を使用して DORA Four Keys メトリクスを計測するCLIツール。

## DORA Four Keys とは

DevOps Research and Assessment (DORA) が定義した、ソフトウェアデリバリーのパフォーマンスを測定する4つの主要指標。

| メトリクス | 説明 | 本ツールでの定義 |
|-----------|------|------------------|
| **Deployment Frequency** | 本番環境へのデプロイ頻度 | mainブランチへのマージ回数 |
| **Lead Time for Changes** | コミットから本番デプロイまでの時間 | PRの最初のコミット → マージまでの時間 |
| **Change Failure Rate** | デプロイによる障害発生率 | hotfix/bugfix PR + bugラベル + revertの割合 |
| **Time to Restore Service** | 障害からの復旧時間 | ※本ツールでは計測対象外 |

### 追加メトリクス

| メトリクス | 説明 |
|-----------|------|
| **Time to First Review** | PR作成から最初のレビューまでの時間 |

## セットアップ

### 1. ビルド

```bash
go build -o dora-metrics .
```

### 2. 環境変数の設定

```bash
cp .env.example .env
vim .env
```

### 3. GitHub Token の取得

[GitHub Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens) から、以下の権限を持つトークンを作成:

- `repo` (プライベートリポジトリの場合)
- `public_repo` (パブリックリポジトリのみの場合)

## 設定項目

### .env ファイル

```bash
# GitHub API Token (必須)
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx

# GitHub Organization or User (必須)
GITHUB_OWNER=your-org

# リポジトリ名 (カンマ区切り、必須)
GITHUB_REPOS=repo1,repo2,repo3

# チームメンバー (カンマ区切り、任意)
# 指定しない場合は全コントリビューターが対象
GITHUB_MEMBERS=user1,user2,user3

# 計測期間 (YYYY-MM-DD形式、必須)
DORA_FROM=2025-01-01
DORA_TO=2025-01-31
```

## 使い方

### 基本

```bash
# .env に設定済みなら期間だけ指定
./dora-metrics --from 2025-01-01 --to 2025-01-31
```

### コマンドライン引数で全て指定

```bash
./dora-metrics \
  --owner your-org \
  --repos repo1,repo2,repo3 \
  --members user1,user2 \
  --from 2025-01-01 \
  --to 2025-01-31 \
  --token ghp_xxxx
```

### 単一リポジトリ

```bash
./dora-metrics \
  --owner your-org \
  --repos single-repo \
  --from 2025-01-01 \
  --to 2025-01-31
```

### 全コントリビューター対象

```bash
# --members を省略すると全員が対象
./dora-metrics \
  --owner your-org \
  --repos repo1 \
  --from 2025-01-01 \
  --to 2025-01-31
```

## オプション一覧

| オプション | 環境変数 | 説明 | 必須 |
|-----------|----------|------|------|
| `--owner` | `GITHUB_OWNER` | GitHub Organization または User | Yes |
| `--repos` | `GITHUB_REPOS` | リポジトリ名 (カンマ区切り) | Yes |
| `--from` | `DORA_FROM` | 開始日 (YYYY-MM-DD) | Yes |
| `--to` | `DORA_TO` | 終了日 (YYYY-MM-DD) | Yes |
| `--members` | `GITHUB_MEMBERS` | フィルタ対象メンバー (カンマ区切り) | No |
| `--token` | `GITHUB_TOKEN` | GitHub API Token | Yes |

## 出力例

```
🚀 DORA Metrics Calculator
   Organization: your-org
   Repositories: [repo1 repo2]
   Members: [user1 user2]
   Period: 2025-01-01 ~ 2025-01-31

📊 Analyzing your-org/repo1...
   Found 23 merged PRs

╔══════════════════════════════════════════════════════════════════╗
║  DORA Metrics: repo1
║  Period: 2025-01-01 ~ 2025-01-31
╠══════════════════════════════════════════════════════════════════╣
║
║  📦 Deployment Frequency
║     Total Deployments: 23
║     Frequency: 0.74 deploys/day (5.2 deploys/week)
║
║  ⏱️  Lead Time for Changes (first commit → merge)
║     Average: 2.3 days
║     Median:  1.1 days
║
║  🔥 Change Failure Rate
║     Failure PRs + Reverts: 2
║     Rate: 8.7%
║
║  👀 Time to First Review (PR created → first review)
║     Average: 5.2 hours
║     Median:  2.8 hours
║
╠══════════════════════════════════════════════════════════════════╣
║  👥 Per-Member Breakdown
╠══════════════════════════════════════════════════════════════════╣
║
║  @user1
║     PRs Merged: 15
║     Lead Time (avg/median): 1.8 days / 0.9 days
║     Time to First Review (avg/median): 4.1 hours / 2.3 hours
║     Failure PRs: 1
║
║  @user2
║     PRs Merged: 8
║     Lead Time (avg/median): 3.2 days / 1.5 days
║     Time to First Review (avg/median): 7.1 hours / 3.5 hours
║     Failure PRs: 1
║
╚══════════════════════════════════════════════════════════════════╝
```

複数リポジトリを指定した場合、最後に Combined Summary も出力されます。

## Change Failure の判定基準

以下のいずれかに該当するPRを障害対応PRとしてカウント:

1. **ブランチ名**: `hotfix` または `bugfix` を含む
2. **ラベル**: `bug`, `hotfix`, `bugfix` を含む
3. **Revertコミット**: mainブランチへの `Revert` で始まるコミット

## 制限事項

- GitHub API のレート制限あり (認証済みで 5,000 req/hour)
- 大量のPRがある場合、API呼び出しに時間がかかる場合があります
- Time to Restore Service (MTTR) は本ツールでは計測していません

## ライセンス

MIT
