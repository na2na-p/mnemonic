# krkrsdl2 パッチ

テンプレートビルド（`.github/workflows/build-android-template.yml`）は
fork（na2na-p/krkrsdl2 の `mnemonic-android` ブランチ）をクローンする。
恒久的な変更は fork へ直接コミットし、このディレクトリの `*.patch` は
一時的な実験にのみ使う（クローン直後に全て適用され、`VERSION` に
`patches=<名前>@<git hash-object>` として記録される）。

パッチファイルは CRLF 改行の krkrsdl2/krkrz ソースへ適用するため、
`.gitattributes` で改行正規化を無効にしている（LF 化されると CI で
`git apply` が失敗する）。

上流（krkrsdl2/krkrsdl2）へ取り込まれたパッチは、このディレクトリから削除すること。

## 関連: メニュー表示時に画面全体が真っ黒になる問題（パッチ対象外）

タイトルメニューが出た瞬間に画面全体が黒くなる症状は krkrsdl2 側ではなく
KAG3 スクリプトと krkrz 系 TJS2 の相互作用が原因（MessageLayer の
`var face;` メンバがネイティブ `face` プロパティをシャドーイングし、
`face = dfProvince` による描画面切替が効かず `colorRect` がメイン画像を
不透明黒で塗る）。この修正は mnemonic 本体の変換処理
（`internal/converter/script.go` の `ApplyMessageLayerCompat`）で行う。
