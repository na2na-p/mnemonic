# TLG6 テストフィクスチャ

`*.tlg` は KiriKiri Z（krkrz）のエンコーダ `visual/SaveTLG6.cpp` の `SaveTLG6`
で生成した TLG6 ファイル、同名の `*.png` はそれを krkrz の参照デコーダ
（`visual/LoadTLG.cpp` の `TVPLoadTLG6` と `visual/tvpgl.c` の
`TVPTLG6DecodeGolombValues*` / `TVPTLG6DecodeLine*` / `TVPTLG5DecompressSlide`）
で復号した期待画素である。

- 参照ソース: <https://github.com/krkrz/krkrz> コミット
  `fd5c4baa6a2ef5978db1bd043634351f48667daf`
  （BSD 系ライセンス。著作権表示と許諾条件はリポジトリ直下の `THIRD_PARTY_NOTICES.md` を参照）
- 入力画像は乱数とグラデーションから合成したもので、第三者の著作物を含まない
- 参照デコーダで復号した画素が入力画像と一致すること（可逆性）を生成時に確認した

| ファイル | 色数 | サイズ | 狙い |
|---|---|---|---|
| `rgba_allfilters_61x29` | 4 | 61x29 | 16 種の色相関フィルタ × MED/平均の全 32 通り、端数ブロック、端数ブロック行 |
| `rgb_allfilters_noise_61x29` | 3 | 61x29 | 上と同じフィルタ網羅をノイズ画像で。ゴロム符号のエスケープ経路を含む |
| `rgba_noise_21x13` | 4 | 21x13 | エンコーダ自身のフィルタ選択、エスケープ経路 |
| `rgba_grad_16x16` | 4 | 16x16 | ブロック境界ちょうどの幅と高さ |
| `rgb_grad_5x3` | 3 | 5x3 | 幅が 1 ブロック未満（端数ブロックのみ） |
| `rgba_noise_1x1` | 4 | 1x1 | 最小サイズ |
| `gray_grad_21x13` | 1 | 21x13 | グレースケール |

全 32 通りのフィルタを網羅する 2 ファイルは、`SaveTLG6` がブロックごとに選ぶ
フィルタ番号と予測方式を、ブロック番号 `fc` から `ft = fc % 16`、
`minp = (fc / 16) % 2` として強制した版で生成した。符号化処理そのものは変えていない。

期待 PNG の画素は、参照デコーダが出力する 32bit 画素（B, G, R, A の順）を
NRGBA へ並べ替えたものである。色数 3 はアルファを 255 とする。色数 1 は参照
デコーダが B に置く輝度を R・G・B へ複製し、アルファを 255 とする。
