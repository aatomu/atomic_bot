# atomic_bot
Golangで自分のために作成  
開発環境:rpi4(8gb) golang(go1.11.6 linux/arm64)  
  
## -起動-  
```go run main.go -prefix=<prefix> -token=<bot token>```
  
## -botｺﾏﾝﾄﾞ-  
### TTS関連  
`<prefix> join` : VCに接続します  
`<prefix> speed <speech speed>` : 読み上げ速度を設定します  
`<prefix> lang <language code || auto>` : 読み上げる言語を変更します  
`<prefix> limit <speech limit>` : 読み上げる文字数の上限を設定します  
`<prefix> leave` : VCから切断します  
  
### Poll関連  
 `!tom poll <質問>,<回答1>,<回答2>...` : 質問を作成します
  
### Role関連  
`!tom role <名前>,@<ロール1>,@<ロール2>...` : ロール管理を作成します  
  *RoleControllerという名前のロールがついている必要があります

## コード元:  
Bot Souce Code : https://github.com/takanakahiko/discord-tts  
Bot API Code   : https://github.com/bwmarrin/discordgo  
Bot Language   : https://golang.org/  
