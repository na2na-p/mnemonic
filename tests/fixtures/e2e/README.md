# E2Eテストフィクスチャ

このディレクトリには、E2Eテスト用のテストデータが格納されています。

## ディレクトリ構造

```
tests/fixtures/e2e/
├── README.md           # このファイル
├── minimal_game/       # E2E-001: 最小構成ビルドテスト用
│   └── .gitkeep
├── convert_game/       # E2E-002: アセット変換ビルドテスト用
│   └── .gitkeep
├── custom_game/        # E2E-003: カスタム設定ビルドテスト用
│   ├── .gitkeep
│   └── mnemonic.yml    # カスタム設定ファイル
└── encrypted.xp3       # E2E-004-2: 暗号化XP3エラーテスト用ダミー
```

## 使用方法

現時点では、E2Eテストは実際のビルド環境（Android SDK、NDK、FFmpeg等）が
必要なため、ほとんどのテストはスキップマークが付いています。

完全なE2E環境をセットアップした後、以下のコマンドでテストを実行できます：

```bash
uv run pytest tests/e2e/ -v -m e2e
```

## テストデータの作成

実際のXP3ファイルは著作権の問題があるため、コミットされていません。
Phase 2でテストデータ生成スクリプト（create_test_data.py）を
実装予定です。

## 参照

