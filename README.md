# Notion × Claude × GitHub Codegen Workflow

> Notion のチケット本文（**toggle「[AI] Spec」配下**）を Claude に渡してコードを生成し、指定の GitHub リポジトリに **ブランチ作成 → 新規ファイル追加 → commit/push → PR 作成** まで自動化します。  
> ブランチ名は **`feature/[unique_id]_AI_Generated`**（Notion の `unique_id` プロパティを使用）。

---

## 目次
- [概要](#概要)
- [要件 / 前提](#要件--前提)
- [セットアップ](#セットアップ)
- [Notion 側の準備](#notion-側の準備)
- [環境変数](#環境変数)
- [使い方](#使い方)
  - [ローカル（Dry-run）](#ローカルdry-run)
  - [ローカル（実PR作成）](#ローカル実pr作成)
  - [GitHub Actions から再利用](#github-actions-から再利用)
- [ブランチ / ファイル命名](#ブランチ--ファイル命名)
- [仕組み](#仕組み)
- [トラブルシュート](#トラブルシュート)
- [セキュリティ注意点](#セキュリティ注意点)
- [今後の拡張（Step2 以降）](#今後の拡張step2-以降)

---

## 概要
このリポジトリは以下を自動で行います。

1. **Notion**: Database から **タグ**でチケットを1件取得（プロパティ `ID` は `unique_id` 型を想定）  
   - ページ本文から **toggle リスト「[AI] Spec」** の**子ブロックのみ**抽出
2. **Claude**: 抽出要件をもとに **純コードのみ**を生成（説明なし／コードフェンス必須）
3. **GitHub**: `feature/[unique_id]_AI_Generated` でブランチ作成  
   `generated/` 以下にファイル追加 → commit/push → **PR 作成**

> ※ 既存コードを参照して差分編集する機能は **Step2** として今後追加予定。

---

## 要件 / 前提
- Node.js **v20 以上**（`--import tsx` を使用）
- Notion Integration（対象 Database に **Connection** で権限付与）
- Anthropic **Claude API Key**
- GitHub **Personal Access Token**（`repo` 権限）— 他リポに push/PR する場合に使用

---

## セットアップ

```bash
# 依存のインストール
npm ci

# .env を作成（Secrets は本番では GitHub Secrets を利用）
cp .env.example .env
```

`.env` の主要項目は後述の「環境変数」を参照してください。

---

## Notion 側の準備
1. **Integration を作成**し、対象 Database の右上 **Share → Connections** で当該 Integration を追加  
2. **Database ID** を URL から取得  
   - 例: `https://www.notion.so/workspace/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx?v=...`  
     → `xxxxxxxx...` が Database ID（ハイフンありでも可）
3. **タグプロパティ**（multi_select / select / status のいずれか）を用意（例: `タグ`）  
   - 生成対象のページに **指定タグ**（例: `ready-for-ai`）を付与
4. ページ本文に **toggle リスト**を作り、**タイトルを `[AI] Spec`** にする  
   - その**子ブロック**に AI に渡したい仕様を書く
5. `unique_id` タイプのプロパティ（例: 名前 `ID`）で **連番などのユニーク値**を持たせる  
   - これをブランチ名に使用します

---

## 環境変数

`.env` または GitHub Secrets に設定します。

| 変数名 | 必須 | 説明 | 例 |
|---|:--:|---|---|
| `NOTION_API_KEY` | ✅ | Notion Integration のシークレット | `secret_xxx` |
| `NOTION_DATABASE_ID` | ✅ | 対象 Database ID | `xxxxxxxx...` |
| `NOTION_TAG_PROP` |  | 抽出に使うタグのプロパティ名 | `タグ` / `Tags` |
| `NOTION_TAG` | ✅ | 抽出タグの値 | `ready-for-ai` |
| `NOTION_UNIQUE_ID_PROP` |  | `unique_id` 型の列名 | `ID` |
| `CLAUDE_API_KEY` | ✅ | Anthropic Claude API Key | `sk-ant-...` |
| `MODEL` |  | Claude モデル | `claude-3-5-sonnet-20240620` |
| `MAX_TOKENS` |  | 生成最大トークン（**整数**） | `800` |
| `LANG_HINT` |  | 生成言語ヒント | `typescript` / `python` |
| `TARGET_REPO` | ✅ | push 先リポ（`org/repo`） | `your-org/target-repo` |
| `BASE_BRANCH` |  | ベースブランチ | `main` |
| `FILE_EXT` |  | 出力拡張子 | `ts` |
| `CODE_PATH` |  | 出力パス | `generated` |
| `GH_PAT` | ✅ | GitHub PAT（`repo` 権限） | `ghp_xxx` |
| `DRY_RUN` |  | `1` で push/PR をスキップ | `1` |
| `USE_MOCK` |  | `1` で Claude をモック | `1` |
| `MAX_DESC_LEN` |  | Notion 本文の最大文字数 | `8000` |
| `AI_TOGGLE_TITLE` |  | toggle タイトル | `[AI] Spec` |

> **注意**: `MAX_TOKENS` は **整数**で設定し、API キーは `CLAUDE_API_KEY` に入れてください。

---

## 使い方

### ローカル（Dry-run）
```bash
export NOTION_TAG="ready-for-ai"
export CLAUDE_API_KEY=sk-ant-xxxx
export GH_PAT=ghp_xxxx
export TARGET_REPO=your-org/sandbox-repo
export DRY_RUN=1

node --import tsx action-src/tools/run_codegen.ts
```
- `payload.json` が生成され、branch/file/PR のプランがログ出力されます。

### ローカル（実PR作成）
```bash
export DRY_RUN=0
node --import tsx action-src/tools/run_codegen.ts
```
- `feature/[unique_id]_AI_Generated` ブランチが作成され、コードファイルが追加され、PRが作成されます。

### GitHub Actions から再利用
呼び出し元リポジトリにワークフローを定義して利用可能です。

```yaml
name: Generate Code from Notion
on:
  workflow_dispatch:
jobs:
  codegen:
    uses: your-org/notion-codegen-workflows/.github/workflows/codegen.yml@v0.2.0
    with:
      notion_tag: "ready-for-ai"
      target_repo: "your-org/target-repo"
      file_ext: "ts"
      dry_run: "false"
    secrets:
      NOTION_API_KEY: ${{ secrets.NOTION_API_KEY }}
      NOTION_DATABASE_ID: ${{ secrets.NOTION_DATABASE_ID }}
      CLAUDE_API_KEY: ${{ secrets.CLAUDE_API_KEY }}
      TARGET_REPO_TOKEN: ${{ secrets.GH_PAT }}
```

---

## ブランチ / ファイル命名
- **ブランチ**: `feature/[unique_id]_AI_Generated`  
  - `unique_id` は Notion の `unique_id` プロパティの値（数値）を文字列化して使用
  - 許容外文字はサニタイズ（空白→`-` など）
- **ファイル**: `generated/[unique_id].<ext>`（`ext` は `FILE_EXT`）

---

## 仕組み
- `action-src/services/notion.ts`  
  - タグでページ1件抽出（プロパティ型に応じて `multi_select/select/status` を自動判定）  
  - 本文ブロックを再帰取得し、**toggle「[AI] Spec」配下のみ**を Markdown に整形  
  - プロパティ `ID`（`unique_id` 型）を読み取り、`uniqueId` として返却
- `action-src/services/claude.ts`  
  - 出力は**コードのみ**になるよう厳格にプロンプト化  
  - 返答から **コードフェンス（```）または `<CODE>` タグ**を抽出し、純コードを返す
- `action-src/services/github.ts`  
  - `feature/[unique_id]_AI_Generated` ブランチを作成  
  - `generated/[unique_id].<ext>` を追加して commit/push  
  - PR を作成（Notion へのリンク・抽出スコープを本文に記載）

---

## トラブルシュート
- **invalid x-api-key (401)**: `CLAUDE_API_KEY` が誤り／期限切れ。`sk-ant-` から始まるキーを設定。  
- **max_tokens エラー (400)**: `MAX_TOKENS` は **整数**で指定。  
- **validation_error: property type mismatch**: Notion のプロパティ型とフィルタが不一致。`print_db_schema` ツールで型を確認。  
- **ブランチ衝突**: 既存ブランチがある場合はタイムスタンプを付けて回避。  
- **ファイル重複**: 既存を上書きしない安全運用（エラー）にしています。必要ならポリシー変更可。

---

## セキュリティ注意点
- API キー（`CLAUDE_API_KEY`/`NOTION_API_KEY`/`GH_PAT`）は **.env をコミットしない**。GitHub Secrets を使用。  
- 生成コードは**必ずレビュー**（権限・外部通信・ライセンス等）。  
- PAT は最小権限・短期有効期限を推奨。

---

## 今後の拡張（Step2 以降）
- 既存コードの参照と**差分編集（パッチ適用）**  
- 生成後の **lint / tsc / unit test** を PR に自動レポート  
- 複数ファイル生成（`<!-- BEGIN CODEFILE:path -->` 形式）への拡張  
- 失敗時の通知（Slack / GitHub コメント）
