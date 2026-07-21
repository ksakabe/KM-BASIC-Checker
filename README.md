# KM-BASIC Syntax Checker

KM-BASICソースコードをMachiKania実機へ転送する前に検査する、移植性重視のコマンドラインツールです。

## 方針

- Go標準ライブラリだけで実装
- Windows、macOS、Linux、FreeBSDなどへ単一バイナリとして配布可能
- ソースコードは読み取り専用で処理し、元ファイルを書き換えない
- 終了コードをCIやエディタから利用可能
- 将来、命令シグネチャと式の型検査を追加できる構成

## 検査する項目

- 文字列リテラルの閉じ忘れ
- `REM`コメント（行または文の残り全体）と、使用できない`'`コメント
- `FOR` / `NEXT`
- `WHILE` / `WEND`
- `DO` / `LOOP`
- 複数行 `IF` / `ELSEIF` / `ELSE` / `ENDIF`
- `FOR`制御変数が整数型か
- ラベルと行番号の重複
- `GOTO`、`GOSUB`、`RESTORE`、`SOUND`の未定義参照
- `USEVAR`、`VAR`の変数名
- 2文字以上の変数が使用前に`USEVAR`で宣言されていること
- 予約語を変数名・ラベル名に使っていないか
- 対象機種に応じた6文字制限の一部
- KM-BASICに実装されていない命令・関数名
- 式の構文、演算子、代入、配列添字、主要関数の整数・実数・文字列型
- 実数乱数を返す拡張関数`RND#()`
- 命令ごとの必須・省略可能・可変長引数の個数と型
- クラスライブラリへの対応

## 実行ファイル

RelaseにWindows、Linux、macOS、FreeBSD向けの実行ファイルがあります。ダウンロートして利用できます。


## ビルド

Goをインストール後、プロジェクトのルートで実行します。

```sh
go test ./...
go build -o kmbasic-check ./cmd/kmbasic-check
```

Windowsでは次のように実行します。

```powershell
go build -o kmbasic-check.exe ./cmd/kmbasic-check
```

## 使用方法

```sh
./kmbasic-check PROGRAM.BAS
```

対象機種を指定する場合:

```sh
./kmbasic-check -target type-pu PROGRAM.BAS
./kmbasic-check -target type-m PROGRAM.BAS
```

クラスライブラリの場所を指定する場合:

```sh
./kmbasic-check -lib /path/to/machikania-p/LIB PROGRAM.BAS
```

macOS版CLIの既定値は`/Users/sakabe/Downloads/MachiKania/1.7.0/machikania-p/LIB`です。

JSON出力:

```sh
./kmbasic-check -format json PROGRAM.BAS
```

## 終了コード

- `0`: エラーなし
- `1`: 構文エラーあり
- `2`: ファイル入出力またはオプションのエラー

## クロスコンパイル例

```sh
# Windows 64bit
GOOS=windows GOARCH=amd64 go build -o dist/kmbasic-check-windows-amd64.exe ./cmd/kmbasic-check

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dist/kmbasic-check-darwin-arm64 ./cmd/kmbasic-check

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o dist/kmbasic-check-darwin-amd64 ./cmd/kmbasic-check

# Linux 64bit
GOOS=linux GOARCH=amd64 go build -o dist/kmbasic-check-linux-amd64 ./cmd/kmbasic-check

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o dist/kmbasic-check-linux-arm64 ./cmd/kmbasic-check

# FreeBSD 64bit
GOOS=freebsd GOARCH=amd64 go build -o dist/kmbasic-check-freebsd-amd64 ./cmd/kmbasic-check
```

## 安全性

このツールはソースコードを実行しません。字句・構造だけを解析します。
ただし、信頼できないファイルを処理する場合に備え、1行の最大読込サイズを1 MiBに制限しています。
CIでは最小権限のユーザーで実行してください。
