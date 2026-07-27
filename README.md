# Mnemonic

吉里吉里2製Windowsゲーム（exe/xp3）をAndroid APKに変換するCLIツール。

## 必要要件

ビルド・実行環境に以下が必要:

| ツール | 用途 |
| --- | --- |
| Go 1.26+ | ツール自体のビルド |
| FFmpeg | 動画/音声変換 |
| Android SDK（Platform 34, NDK r21） | APKビルド |
| Java JDK 17+ | Gradle実行 |
| Gradle | Androidプロジェクトのビルド |

依存ツールが揃っているかは `mnemonic doctor` で確認できる。

## インストール

### go install を使う場合

```bash
go install github.com/na2na-p/mnemonic/cmd/mnemonic@latest
```

### ソースからビルドする場合

```bash
git clone https://github.com/na2na-p/mnemonic.git
cd mnemonic
go build -o mnemonic ./cmd/mnemonic
```

## 使い方

### APKビルド

```bash
mnemonic build <input.exe> -o <output.apk>
```

主なオプション:

```
  -o, --output string               出力APKパス
      --app-name string             アプリ表示名
      --package-name string         Androidパッケージ名
      --keystore string             署名用キーストア
      --quality string              画像品質プリセット (default "high")
      --skip-video                  動画変換をスキップ
      --clean                       キャッシュをクリア
      --template-version string     テンプレートバージョン固定
      --template-refresh-days int   テンプレートキャッシュ期限（日）(default 7)
      --template-offline            オフラインモード
      --ffmpeg-timeout int          FFmpegタイムアウト（秒）(default 300)
      --gradle-timeout int          Gradleタイムアウト（秒）(default 1800)
      --log-file string             ログファイル出力先
  -v, --verbose count               詳細ログ出力
```

`--keystore` 指定時、署名パスワードは環境変数 `MNEMONIC_KEYSTORE_PASS` から読み込む
（設定されていれば対話入力を求めない）。CI等の非対話実行では必ずこの環境変数を
設定すること。

### 依存ツールのチェック

```bash
mnemonic doctor
```

### ゲーム構成の解析表示

```bash
mnemonic info <input>
```

### キャッシュ管理

```bash
mnemonic cache info    # キャッシュ情報を表示
mnemonic cache clean   # キャッシュを削除
```

各コマンドの詳細は `mnemonic <command> --help` で確認できる。

## 開発

```bash
# ビルド
go build ./...

# テスト（CIと同等: race + shuffle）
go test -race -shuffle=on ./...

# Lint / Format（golangci-lint v2）
golangci-lint run
golangci-lint fmt
```

設計の詳細は [`docs/designdoc.md`](docs/designdoc.md) を参照。
