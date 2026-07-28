# krkrsdl2 パッチ

テンプレートビルド（`.github/workflows/build-android-template.yml`）が krkrsdl2 を
クローンした直後に、このディレクトリの `*.patch` を全て適用してからビルドする。
適用されたパッチはテンプレートの `VERSION` に `patches=<名前>@<git hash-object>` として記録される。

パッチファイルは CRLF 改行の krkrsdl2/krkrz ソースへ適用するため、
`.gitattributes` で改行正規化を無効にしている（LF 化されると CI で
`git apply` が失敗する）。

上流（krkrsdl2/krkrsdl2）へ取り込まれたパッチは、このディレクトリから削除すること。

## krkrsdl2-android-zoom.patch

**症状**: Android 実機・エミュレータで、640x480 のゲームが画面左上に原寸で
押し込まれ、残りの領域が黒いまま表示される（スケーリングもレターボックス
配置もされない）。

**原因**: Android では `KRKRSDL2_WINDOW_SIZE_IS_LAYER_SIZE` が定義され、ウィンドウは
`SDL_WINDOW_FULLSCREEN_DESKTOP` で端末解像度いっぱいに作られる。このとき
`KRKRSDL2_ENABLE_ZOOM` が無効だと `GetInnerWidth()/GetInnerHeight()` が
ウィンドウの実サイズ（例: 1080x2424）をそのまま返すため、ゲーム解像度への
スケーリングが行われない。また ZOOM を有効にしただけでは、このプラットフォーム
では `SetInnerSize` が呼ばれないため `InnerWidth/InnerHeight` が初期値 32 の
ままになり、描画元矩形が壊れる。

**修正**: Android で `KRKRSDL2_ENABLE_ZOOM` を有効化し、`SetPaintBoxSize` で
`InnerWidth/InnerHeight` をレイヤサイズに同期する。これにより
`UpdateActualZoom()` がアスペクト比を維持したレターボックス配置と
スケーリングを行う。

## 関連: メニュー表示時に画面全体が真っ黒になる問題（パッチ対象外）

タイトルメニューが出た瞬間に画面全体が黒くなる症状は krkrsdl2 側ではなく
KAG3 スクリプトと krkrz 系 TJS2 の相互作用が原因（MessageLayer の
`var face;` メンバがネイティブ `face` プロパティをシャドーイングし、
`face = dfProvince` による描画面切替が効かず `colorRect` がメイン画像を
不透明黒で塗る）。この修正は mnemonic 本体の変換処理
（`internal/converter/script.go` の `ApplyMessageLayerCompat`）で行う。
