# Go HTTPクライアント Content-Length/Transfer-Encoding周りの挙動の実験プログラム
ファイルを送信するHTTPリクエストを様々な設定のもとで実行し、リクエストの内容を観察・比較

## Usage
```bash
go run main.go -f <filename>
```

## リクエスト設定一覧
- `Request.ContentLength`をセットしない
- `Request.ContentLength`をセットする(正しい`Content-Length`の設定方法)
- `Request.Header`に`Content-Length`をセットする(間違った`Content-Length`の設定方法)
- ファイルの内容を一旦`bytes.Buffer`にコピーしたうえでリクエスト
- ファイルの内容を一旦`bytes.Buffer`にコピーし、さらに`Request.TransferEncoding`に`chunked`をセットしてリクエスト
- `mime/multipart`を利用したマルチパートリクエスト


## 詳細
素のTCPサーバを立ててHTTPリクエストをダンプする方法を採っている。他の方法には以下の問題がある:
- `httputil.DumpRequest`では`Content-Length`の挙動を確認できない
- `net/http`に基づくサーバでは`Transfer-Encoding: chunked`で送信されたリクエストの内容を正確に把握できない
