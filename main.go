package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/url"
	"time"

	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/atomu21263/atomicgo"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/language"
)

type SessionData struct {
	guildID     string
	channelID   string
	vcsession   *discordgo.VoiceConnection
	speechLimit int
	speechLang  string
	mut         sync.Mutex
	enableBot   bool
}

type UserSetting struct {
	lang  string
	speed float64
	pitch float64
}

var (
	//変数定義
	prefix   = flag.String("prefix", "", "call prefix")
	token    = flag.String("token", "", "bot token")
	clientID = ""
	sessions = atomicgo.ExMapGet()
)

func main() {
	//flag入手
	flag.Parse()
	fmt.Println("prefix       :", *prefix)
	fmt.Println("token        :", *token)

	//bot起動準備
	discord := atomicgo.DiscordBotSetup(*token)

	//eventトリガー設定
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onVoiceStateUpdate)
	discord.AddHandler(onMessageReactionAdd)
	discord.AddHandler(onMessageReactionRemove)

	//起動
	atomicgo.DiscordBotStart(discord)
	defer func() {
		sessions.Range(func(key interface{}, value interface{}) bool {
			atomicgo.SendEmbed(discord, value.(*SessionData).channelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "__Infomation__",
				Description: "Sorry. Bot will  Shutdown. Will be try later.",
				Color:       0xff00ff,
			})
			return true
		})
		atomicgo.DiscordBotEnd(discord)
	}()
	//起動メッセージ表示
	fmt.Println("Listening...")

	//bot停止対策
	atomicgo.StopWait()
}

//BOTの準備が終わったときにCall
func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID
	//1秒に1回呼び出す
	oneSecTicker := time.NewTicker(1 * time.Second)
	tenSecTicker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
			case <-oneSecTicker.C:
				botStateUpdate(discord)
			case <-tenSecTicker.C:
				serverInfoUpdate(discord)
			}
		}
	}()
}

func botStateUpdate(discord *discordgo.Session) {
	//botのステータスアップデート
	joinedServer := len(discord.State.Guilds)
	joinedVC := 0
	sessions.Range(func(key interface{}, value interface{}) bool {
		joinedVC++
		return true
	})
	VC := ""
	if joinedVC != 0 {
		VC = " " + strconv.Itoa(joinedVC) + "鯖でお話し中"
	}
	state := discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: *prefix + " help | " + strconv.Itoa(joinedServer) + "鯖で稼働中" + VC,
				Type: 0,
			},
		},
		AFK:    false,
		Status: "online",
	}
	discord.UpdateStatusComplex(state)
}

func serverInfoUpdate(discord *discordgo.Session) {
	joinedGuilds, _ := discord.UserGuilds(100, "", "")
	for _, guild := range joinedGuilds {
		guildChannels, err := discord.GuildChannels(guild.ID)
		if atomicgo.PrintError("Failed get GuildChannels", err) {
			continue
		}

		//Info カテゴリーチェック
		categoryID := ""
		for _, channel := range guildChannels {
			if channel.Name == "Server Info" && channel.Type == 4 {
				categoryID = channel.ID
				break
			}
		}

		//ないならreturn
		if categoryID == "" {
			continue
		}

		//更新
		for _, channel := range guildChannels {
			if channel.ParentID == categoryID {
				switch {
				//すべて
				case strings.HasPrefix(channel.Name, "User:"):
					guild, err := discord.State.Guild(channel.GuildID)
					if atomicgo.PrintError("Failed get GuildData", err) {
						continue
					}

					name := "User: " + strconv.Itoa(guild.MemberCount)
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//ロール数
				case strings.HasPrefix(channel.Name, "Role:"):
					guild, _ := discord.State.Guild(channel.GuildID)
					if atomicgo.PrintError("Failed get GuildData", err) {
						continue
					}

					//@everyoneも入ってるから-1
					name := "Role: " + strconv.Itoa(len(guild.Roles)-1)
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//絵文字
				case strings.HasPrefix(channel.Name, "Emoji:"):
					guild, err := discord.State.Guild(channel.GuildID)
					if atomicgo.PrintError("Failed get GuildData", err) {
						continue
					}

					name := "Emoji: " + strconv.Itoa(len(guild.Emojis))
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//チャンネル数
				case strings.HasPrefix(channel.Name, "Channel:"):
					guild, err := discord.State.Guild(channel.GuildID)
					if atomicgo.PrintError("Failed get GuildData", err) {
						continue
					}

					count := 0
					for _, channel := range guild.Channels {
						if channel.Type != 4 && channel.ID != categoryID && channel.ParentID != categoryID {
							count++
						}
					}
					name := "Channel: " + strconv.Itoa(count)
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
				}
			}
		}
	}
}

//メッセージが送られたときにCall
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	mData := atomicgo.MessageViewAndEdit(discord, m)

	// 読み上げ無し のチェック
	if strings.HasPrefix(m.Content, ";") {
		return
	}

	switch {
	//TTS関連
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" join"):
		ok := sessions.ExMapCheck(mData.GuildID)
		if ok {
			atomicgo.PrintError("VC joined "+mData.GuildID+" join duplicate", fmt.Errorf("fined this server voice chat"))
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		joinVoiceChat(mData.ChannelID, mData.GuildID, discord, mData.UserID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" get"):
		viewUserSetting(mData.UserID, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" speed "):
		changeUserSpeed(mData.UserID, mData.Message, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" pitch "):
		changeUserPitch(mData.UserID, mData.Message, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" lang "):
		changeUserLang(mData.UserID, mData.Message, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" limit "):
		session, ok := sessions.ExMapLoad(mData.GuildID)
		if !ok || session.(*SessionData).channelID != mData.ChannelID {
			atomicgo.PrintError("VC non fined in "+mData.GuildID, fmt.Errorf("not fined this server voice chat"))
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		changeSpeechLimit(session.(*SessionData), mData.Message, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" word "):
		addWord(mData.Message, mData.GuildID, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" bot"):
		session, ok := sessions.ExMapLoad(mData.GuildID)
		if !ok || session.(*SessionData).channelID != mData.ChannelID {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		session.(*SessionData).enableBot = !session.(*SessionData).enableBot
		atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🤖")
		if session.(*SessionData).enableBot {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🔈")
		} else {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🔇")
		}
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" leave"):
		session, ok := sessions.ExMapLoad(mData.GuildID)
		if !ok || session.(*SessionData).channelID != mData.ChannelID {
			atomicgo.PrintError("Failed Leave VC OR no reading channel in "+mData.GuildID, fmt.Errorf("not fined this server voice chat"))
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		leaveVoiceChat(session.(*SessionData), discord, mData.ChannelID, mData.MessageID, true)
		return
		//Poll関連
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" poll "):
		createPoll(mData.Message, mData.UserName, discord, mData.ChannelID, mData.MessageID)
		return
	//Role関連
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" role "):
		if atomicgo.HaveRole(discord, mData.GuildID, mData.UserID, "RoleController") {
			crateRoleManager(mData.Message, mData.UserName, discord, mData.ChannelID, mData.MessageID)
			return
		}
		atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
		return
	//info
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" info"):
		if atomicgo.HaveRole(discord, mData.GuildID, mData.UserID, "InfoController") {
			serverInfo(discord, mData.GuildID, mData.ChannelID, mData.MessageID)
			return
		}
		atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
		return
	case atomicgo.StringCheck(mData.Message, `^https://(canary\.discord\.com|discord\.com)/channels/[0-9]+/[0-9]+/[0-9]+$`):
		data := regexp.MustCompile(`https://(canary\.discord\.com|discord\.com)/channels/`).ReplaceAllString(mData.Message, "")
		IDs := strings.Split(data, "/")
		tranceGuild, err := discord.Guild(IDs[0])
		atomicgo.PrintError("GuildID to Struct", err)
		tranceChannel, err := discord.Channel(IDs[1])
		atomicgo.PrintError("ChannelID to Struct", err)
		tranceMessage, err := discord.ChannelMessage(IDs[1], IDs[2])
		atomicgo.PrintError("ChannelMessage to Struct", err)
		if err != nil {
			return
		}
		//embedのData作成
		embed := &discordgo.MessageEmbed{
			URL:         mData.Message,
			Type:        "rich",
			Description: tranceMessage.Content,
			Timestamp:   tranceMessage.Timestamp.GoString(),
			Color:       0xFFFFFF,
			Author: &discordgo.MessageEmbedAuthor{
				Name:    tranceMessage.Author.Username,
				IconURL: tranceMessage.Author.AvatarURL("128"),
			},
			Footer: &discordgo.MessageEmbedFooter{
				IconURL: tranceGuild.IconURL(),
				Text:    tranceChannel.Name,
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "URL:",
					Value: mData.Message,
				},
			},
		}
		//送信
		atomicgo.SendEmbed(discord, mData.ChannelID, embed)
	//help
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" help"):
		sendHelp(discord, mData.ChannelID)
		return
		//その他
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" debug") && mData.UserID == "701336137012215818":
		if atomicgo.StringCheck(mData.Message, "[0-9]$") {
			guildID := atomicgo.StringReplace(mData.Message, "", "^"+*prefix+` debug\s*`)
			log.Println("Deleting SessionItem : " + guildID)
			sessions.ExMapDelete(mData.GuildID)
			return
		}
		sessions.Range(func(key interface{}, value interface{}) bool {
			guild, err := discord.Guild(value.(*SessionData).guildID)
			if atomicgo.PrintError("Failed Get GuildData by GuildID", err) {
				return true
			}
			channel, err := discord.Channel(value.(*SessionData).channelID)
			if atomicgo.PrintError("Failed Get ChannelData by ChannelID", err) {
				return true
			}
			atomicgo.SendEmbed(discord, mData.ChannelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "DebugData\nGuild:" + guild.Name + "(" + value.(*SessionData).guildID + ")\nChannel:" + channel.Name + "(" + value.(*SessionData).channelID + ")",
				Description: fmt.Sprintf("```%#v```", value.(*SessionData).vcsession),
				Color:       0xff00ff,
			})

			return true
		})
	}

	//読み上げ
	session, ok := sessions.ExMapLoad(mData.GuildID)
	if ok && session.(*SessionData).channelID == mData.ChannelID && !(m.Author.Bot && !session.(*SessionData).enableBot) {
		speechOnVoiceChat(mData.UserID, session.(*SessionData), mData.Message)
		return
	}

}

func joinVoiceChat(channelID string, guildID string, discord *discordgo.Session, userID string, messageID string) {
	voiceConection, err := atomicgo.JoinUserVCchannel(discord, userID)
	if atomicgo.PrintError("Failed join vc", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	session := &SessionData{
		guildID:     guildID,
		channelID:   channelID,
		vcsession:   voiceConection,
		speechLimit: 50,
		speechLang:  "auto",
		mut:         sync.Mutex{},
	}
	sessions.ExMapWrite(guildID, session)
	atomicgo.AddReaction(discord, channelID, messageID, "✅")
	speechOnVoiceChat("BOT", session, "おはー")
}

func speechOnVoiceChat(userID string, session *SessionData, text string) {
	data, err := os.Open("./dic/" + session.guildID + ".txt")
	if atomicgo.PrintError("Failed open dictionary", err) {
		//フォルダがあるか確認
		_, err := os.Stat("./dic")
		//フォルダがなかったら作成
		if os.IsNotExist(err) {
			err = os.Mkdir("./dic", 0777)
			atomicgo.PrintError("Failed create directory", err)
		}
		//ふぁいる作成
		ok := atomicgo.WriteFileFlash("./dic/"+session.guildID+".txt", []byte{}, 0777)
		if !ok {
			atomicgo.PrintError("Failed create dictionary", fmt.Errorf("permission denied?"))
		}
	}
	defer data.Close()

	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Text()
		replace := regexp.MustCompile(`,.*`)
		from := replace.ReplaceAllString(line, "")
		replace = regexp.MustCompile(`.*,`)
		to := replace.ReplaceAllString(line, "")
		text = strings.ReplaceAll(text, from, to)
	}

	if regexp.MustCompile(`<a:|<:|<@|<#|<@&|http|` + "```").MatchString(text) {
		text = "すーきっぷ"
	}

	//! ? { } < >を読み上げない
	replace := regexp.MustCompile(`!|\?|{|}|<|>|`)
	text = replace.ReplaceAllString(text, "")

	settingData, err := userConfig(userID, UserSetting{})
	atomicgo.PrintError("Failed func userConfig()", err)

	if settingData.lang == "auto" {
		settingData.lang = "ja"
		if regexp.MustCompile(`^[a-zA-Z0-9\s.,]+$`).MatchString(text) {
			settingData.lang = "en"
		}
	}

	//改行停止
	if strings.Contains(text, "\n") {
		replace := regexp.MustCompile(`\n.*`)
		text = replace.ReplaceAllString(text, "")
	}

	//隠れてるところを読み上げない
	if strings.Contains(text, "||") {
		replace := regexp.MustCompile(`\|\|.*\|\|`)
		text = replace.ReplaceAllString(text, "ピーーーー")
	}

	//text cut
	limit := session.speechLimit
	nowCount := 0
	read := ""
	for _, text := range strings.Split(text, "") {
		if nowCount < limit {
			read = read + text
			nowCount++
		}
	}

	//読み上げ待機
	session.mut.Lock()
	defer session.mut.Unlock()

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%s&tl=%s", url.QueryEscape(read), settingData.lang)
	err = atomicgo.PlayAudioFile(settingData.speed, settingData.pitch, session.vcsession, voiceURL)
	atomicgo.PrintError("Failed play Audio \""+read+"\" ", err)
}

func viewUserSetting(userID string, discord *discordgo.Session, channelID string, messageID string) {
	settingData, err := userConfig(userID, UserSetting{})
	if atomicgo.PrintError("Failed func userConfig()", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}
	//embedのData作成
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "",
		Description: "",
		Color:       1000,
	}
	userData, err := discord.User(userID)
	if atomicgo.PrintError("Failed get UserData", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}
	embed.Title = "@" + userData.Username + "'s Speech Config"
	embedText := "Lang:\n" +
		settingData.lang + "\n" +
		"Speed:\n" +
		fmt.Sprint(settingData.speed) + "\n" +
		"Pitch:\n" +
		fmt.Sprint(settingData.pitch)
	embed.Description = embedText
	//送信
	if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
		atomicgo.PrintError("Failed send Embed", err)
	}
}

func changeUserSpeed(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	speedText := strings.Replace(message, *prefix+" speed ", "", 1)

	speed, err := strconv.ParseFloat(speedText, 64)
	if atomicgo.PrintError("Failed speed string to float64", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	if speed < 0.5 || 5 < speed {
		atomicgo.PrintError("Speed is too fast or too slow.", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	_, err = userConfig(userID, UserSetting{
		speed: speed,
	})
	if atomicgo.PrintError("Failed write speed", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}
	atomicgo.AddReaction(discord, channelID, messageID, "🔊")
}

func changeUserPitch(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	pitchText := strings.Replace(message, *prefix+" pitch ", "", 1)

	pitch, err := strconv.ParseFloat(pitchText, 64)
	if atomicgo.PrintError("Failed pitch string to float64", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	if pitch < 0.5 || 1.5 < pitch {
		atomicgo.PrintError("Pitch is too high or too low.", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	_, err = userConfig(userID, UserSetting{
		pitch: pitch,
	})
	if atomicgo.PrintError("Failed write pitch", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}
	atomicgo.AddReaction(discord, channelID, messageID, "🎶")
}

func changeUserLang(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	lang := strings.Replace(message, *prefix+" lang ", "", 1)

	if lang == "auto" {
		_, err := userConfig(userID, UserSetting{
			lang: lang,
		})
		if atomicgo.PrintError("Failed write lang", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
		atomicgo.AddReaction(discord, channelID, messageID, "🗣️")
		return
	}

	_, err := language.Parse(lang)
	if atomicgo.PrintError("Lang is unknownLanguage", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	_, err = userConfig(userID, UserSetting{
		lang: lang,
	})
	if atomicgo.PrintError("Failed write lang", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	atomicgo.AddReaction(discord, channelID, messageID, "🗣️")
}

func userConfig(userID string, user UserSetting) (result UserSetting, err error) {
	//BOTチェック
	if userID == "BOT" {
		return UserSetting{
			lang:  "ja",
			speed: 1.75,
			pitch: 1,
		}, nil
	}

	//ファイルパスの指定
	fileName := "./UserConfig.txt"

	if !atomicgo.CheckFile(fileName) {
		if !atomicgo.CreateFile(fileName) {
			return UserSetting{}, fmt.Errorf("failed Create Config File")
		}
	}
	byteText, ok := atomicgo.ReadFile(fileName)
	if !ok {
		return UserSetting{}, fmt.Errorf("failed Read Config File")
	}
	text := string(byteText)
	//UserIDからデータを入手
	writeText := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "UserID:"+userID) {
			fmt.Sscanf(line, "UserID:"+userID+" Lang:%s Speed:%f Pitch:%f", &result.lang, &result.speed, &result.pitch)
		} else {
			if line != "" && !atomicgo.StringCheck(line, "^UserID:.* Lang:auto Speed:1 Pitch:1$") {
				writeText = writeText + line + "\n"
			}
		}
	}

	//書き込みチェック用変数
	shouldWrite := false
	// チェック用
	nilUserSetting := UserSetting{}
	//上書き もしくはデータ作成
	// user   が !nil とき 書き込み
	if user != nilUserSetting {
		shouldWrite = true
	}

	// result が  nil とき 書き込み
	if result == nilUserSetting {
		shouldWrite = true
		result = UserSetting{
			lang:  "auto",
			speed: 1.0,
			pitch: 1.0,
		}
	}

	// 書き込み
	if shouldWrite {
		//lang
		if user.lang != "" {
			result.lang = user.lang
		}
		//speed
		if user.speed != 0.0 {
			result.speed = user.speed
		}
		//pitch
		if user.pitch != 0 {
			result.pitch = user.pitch
		}
		//最後に書き込むテキストを追加(Write==trueの時)
		writeText = writeText + fmt.Sprintf("UserID:%s Lang:%s Speed:%.2f Pitch:%.2f", userID, result.lang, result.speed, result.pitch)
		//書き込み
		atomicgo.WriteFileFlash(fileName, []byte(writeText), 0777)
		log.Println("User userConfig Writed")
	}
	return
}

func changeSpeechLimit(session *SessionData, message string, discord *discordgo.Session, channelID string, messageID string) {
	limitText := strings.Replace(message, *prefix+" limit ", "", 1)

	limit, err := strconv.Atoi(limitText)
	if atomicgo.PrintError("Faliled limit string to int", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	if limit <= 0 || 100 < limit {
		atomicgo.PrintError("Limit is too most or too lowest.", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	session.speechLimit = limit
	atomicgo.AddReaction(discord, channelID, messageID, "🥺")
}

func addWord(message string, guildID string, discord *discordgo.Session, channelID string, messageID string) {
	text := strings.Replace(message, *prefix+" word ", "", 1)

	if !atomicgo.StringCheck(text, "^.+?,.+?$") {
		err := fmt.Errorf(text)
		atomicgo.PrintError("Check failed word", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//ファイルの指定
	fileName := "./dic/" + guildID + ".txt"
	//dirがあるか確認
	if !atomicgo.CheckFile("./dic/") {
		if !atomicgo.CreateDir("./dic/", 0775) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
	}
	//fileがあるか確認
	if !atomicgo.CheckFile(fileName) {
		if !atomicgo.CreateFile(fileName) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
	}
	textByte, _ := atomicgo.ReadFile(fileName)
	dic := string(textByte)

	//textをfrom toに
	from := ""
	to := ""
	_, err := fmt.Sscanf(strings.ReplaceAll(text, ",", " "), "%s %s", &from, &to)
	if atomicgo.PrintError("Failed message to dic in addWord()", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//確認
	if strings.Contains(dic, "\n"+from+",") {
		text = atomicgo.StringReplace(text, "\n", "\n"+from+",.+?\n")
	}

	dic = dic + text + "\n"
	//書き込み
	ok := atomicgo.WriteFileFlash(fileName, []byte(dic), 0777)
	if !ok {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	atomicgo.AddReaction(discord, channelID, messageID, "📄")
}

func leaveVoiceChat(session *SessionData, discord *discordgo.Session, channelID string, messageID string, reaction bool) {
	speechOnVoiceChat("BOT", session, "さいなら")

	err := session.vcsession.Disconnect()
	if atomicgo.PrintError("Try disconect is Failed", err) {
		if reaction {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
		}
	} else {
		if reaction {
			atomicgo.AddReaction(discord, channelID, messageID, "⛔")
		}
	}
	sessions.ExMapDelete(session.guildID)
}

func createPoll(message string, author string, discord *discordgo.Session, channelID string, messageID string) {
	//複数?あるか確認
	if !strings.Contains(message, ",") {
		log.Println("unknown word")
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//長さ確認
	replace := regexp.MustCompile(*prefix + " poll|,$")
	poll := replace.ReplaceAllString(message, "")
	text := strings.Split(poll, ",")
	//Title+Questionだから-1
	length := len(text) - 1
	if length <= 20 {
		//embedとかreaction用のやつ
		alphabet := []string{"", "🇦", "🇧", "🇨", "🇩", "🇪", "🇫", "🇬", "🇭", "🇮", "🇯", "🇰", "🇱", "🇲", "🇳", "🇴", "🇵", "🇶", "🇷", "🇸", "🇹"}
		//embedのData作成
		embed := &discordgo.MessageEmbed{
			Type:        "rich",
			Title:       "",
			Description: "",
			Color:       1000,
			Footer:      &discordgo.MessageEmbedFooter{Text: "Poller"},
			Author:      &discordgo.MessageEmbedAuthor{Name: ""},
		}
		//作成者表示
		embed.Author.Name = "create by @" + author
		//Titleの設定
		embed.Title = text[0]
		//中身の設定
		Question := ""
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + " : " + text[i] + "\n"
		}
		embed.Description = Question
		//送信
		message, err := discord.ChannelMessageSendEmbed(channelID, embed)
		if atomicgo.PrintError("Failed send Embed", err) {
			return
		}

		//リアクションと中身の設定
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + text[i] + "\n"
			atomicgo.AddReaction(discord, channelID, message.ID, alphabet[i])
		}
	} else {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
	}
}

func crateRoleManager(message string, author string, discord *discordgo.Session, channelID string, messageID string) {
	//複数?あるか確認
	if !strings.Contains(message, ",") {
		err := fmt.Errorf(message)
		atomicgo.PrintError("Check failed message contains", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//roleが指定されてるか確認
	if !strings.Contains(message, "<@&") {
		err := fmt.Errorf(message)
		atomicgo.PrintError("Check failed message contains", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//長さ確認
	replace := regexp.MustCompile(*prefix + " role|,$")
	role := replace.ReplaceAllString(message, "")
	text := strings.Split(role, ",")
	//Title+Questionだから-1
	length := len(text) - 1
	if length <= 20 {
		//embedとかreaction用のやつ
		alphabet := []string{"", "🇦", "🇧", "🇨", "🇩", "🇪", "🇫", "🇬", "🇭", "🇮", "🇯", "🇰", "🇱", "🇲", "🇳", "🇴", "🇵", "🇶", "🇷", "🇸", "🇹"}
		//embedのData作成
		embed := &discordgo.MessageEmbed{
			Type:        "rich",
			Title:       "",
			Description: "",
			Footer:      &discordgo.MessageEmbedFooter{Text: "RoleContoler"},
			Color:       1000,
			Author:      &discordgo.MessageEmbedAuthor{Name: ""},
		}
		//作成者表示
		embed.Author.Name = "create by @" + author
		//Titleの設定
		embed.Title = text[0]
		//中身の設定
		Question := ""
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + " : " + text[i] + "\n"
		}
		embed.Description = Question
		//送信
		message, err := discord.ChannelMessageSendEmbed(channelID, embed)
		if atomicgo.PrintError("Failed send Embed", err) {
			return
		}
		//リアクションと中身の設定
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + text[i] + "\n"
			atomicgo.AddReaction(discord, channelID, message.ID, alphabet[i])
		}
	} else {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
	}
}

func serverInfo(discord *discordgo.Session, guildID string, channelID string, messageID string) {
	channels, err := discord.GuildChannels(guildID)
	atomicgo.PrintError("Failed get GuildChannels", err)
	shouldCreateCategory := true
	categoryID := ""
	for _, channelData := range channels {
		if channelData.Name == "Server Info" {
			shouldCreateCategory = false
			categoryID = channelData.ID
		}
	}
	//チャンネル削除
	if !shouldCreateCategory {
		//チャンネル削除
		for _, channelData := range channels {
			if channelData.ParentID == categoryID {
				_, err := discord.ChannelDelete(channelData.ID)
				if atomicgo.PrintError("Failed delete GuildChannel", err) {
					atomicgo.AddReaction(discord, channelID, messageID, "❌")
					return
				}
			}
		}
		//カテゴリー削除
		_, err := discord.ChannelDelete(categoryID)
		if atomicgo.PrintError("Failed get GuildCategory", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
		atomicgo.AddReaction(discord, channelID, messageID, "🛑")
		return
	}

	//チャンネル作成
	if shouldCreateCategory {
		createChannelData := discordgo.GuildChannelCreateData{
			Name:     "Server Info",
			Type:     4,
			Position: 0,
			NSFW:     false,
		}
		//カテゴリー作成
		categoryData, err := discord.GuildChannelCreateComplex(guildID, createChannelData)
		if atomicgo.PrintError("Failed Create GuildCategory", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
		//everyoneロールID
		guildRoleList, _ := discord.GuildRoles(guildID)
		everyoneID := guildRoleList[0].ID
		//チャンネル作成
		//初期設定
		createChannelData = discordgo.GuildChannelCreateData{
			Type: 2,
			PermissionOverwrites: []*discordgo.PermissionOverwrite{
				{
					ID:    everyoneID,
					Type:  0,
					Deny:  1048576,
					Allow: 0,
				},
				{
					ID:    discord.State.User.ID,
					Type:  1,
					Deny:  0,
					Allow: 1048576,
				},
			},
			ParentID: categoryData.ID,
			Position: 0,
		}

		//User
		createChannelData.Name = "User: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if atomicgo.PrintError("Failed create GuildChannel (User)", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}

		//Roles
		createChannelData.Name = "Role: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if atomicgo.PrintError("Failed create GuildChannel (Role)", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}

		//Emoji
		createChannelData.Name = "Emoji: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if atomicgo.PrintError("Failed create GuildChannel (Emoji)", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}

		//Channel
		createChannelData.Name = "Channel: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if atomicgo.PrintError("Failed create GuildChannel (Channel)", err) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}

		atomicgo.AddReaction(discord, channelID, messageID, "📊")
		return
	}
}

func sendHelp(discord *discordgo.Session, channelID string) {
	//embedのData作成
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "BOT HELP",
		Description: "",
		Color:       1000,
	}
	Text := "--TTS--\n" +
		*prefix + " join :VCに参加します\n" +
		*prefix + " get :読み上げ設定を表示します(User単位)\n" +
		*prefix + " speed <0.5-5> : 読み上げ速度を変更します(User単位)\n" +
		*prefix + " pitch <0.5-1.5> : 声の高さを変更します(User単位)\n" +
		*prefix + " lang <言語> : 読み上げ言語を変更します(User単位)\n" +
		*prefix + " word <元>,<先> : 辞書を登録します(Guild単位)\n" +
		*prefix + " limit <1-100> : 読み上げ文字数の上限を設定します(Guild単位)\n" +
		*prefix + " bot : Botのメッセージを読み上げるかをトグルします(Guild単位)\n" +
		*prefix + " leave : VCから切断します\n" +
		"--Poll--\n" +
		*prefix + " poll <質問>,<回答1>,<回答2>... : 質問を作成します\n" +
		"--Role--\n" +
		*prefix + " role <名前>,@<ロール1>,@<ロール2>... : ロール管理を作成します\n" +
		"*RoleControllerという名前のロールがついている必要があります\n" +
		"--ServerInfo--\n" +
		*prefix + " info : サーバーのデータを表示します\n" +
		"*InfoControllerという名前のロールがついている必要があります\n"
	embed.Description = Text
	//送信
	if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
		atomicgo.PrintError("Failed send help Embed", err)
		log.Println(err)
	}
}

//VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {

	//セッションがあるか確認
	ok := sessions.ExMapCheck(v.GuildID)
	if !ok {
		return
	}

	session, _ := sessions.ExMapLoad(v.GuildID)

	//VCに接続があるか確認
	if session.(*SessionData).vcsession == nil {
		return
	}

	// ボイスチャンネルに誰かしらいたら return
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if session.(*SessionData).vcsession.ChannelID == vs.ChannelID && vs.UserID != clientID {
				return
			}
		}
	}

	// ボイスチャンネルに誰もいなかったら Disconnect する
	leaveVoiceChat(session.(*SessionData), discord, "", "", false)
}

//リアクション追加でCall
func onMessageReactionAdd(discord *discordgo.Session, reaction *discordgo.MessageReactionAdd) {
	rData := atomicgo.ReactionAddViewAndEdit(discord, reaction)

	//embedがあるか確認
	if rData.MessageData.Embeds == nil {
		return
	}

	//Roleのやつか確認
	for _, embed := range rData.MessageData.Embeds {
		footerData := embed.Footer
		if footerData == nil || !strings.Contains(embed.Footer.Text, "RoleContoler") {
			return
		}
	}

	//複配列をstringに変換
	text := ""
	for _, embed := range rData.MessageData.Embeds {
		text = text + embed.Description
	}

	//stringを配列にして1個ずつ処理
	for _, embed := range strings.Split(text, "\n") {
		//ロール追加
		if strings.HasPrefix(embed, rData.Emoji) {
			replace := regexp.MustCompile(`[^0-9]`)
			roleID := replace.ReplaceAllString(embed, "")
			err := discord.GuildMemberRoleAdd(rData.GuildID, rData.UserID, roleID)
			//失敗時メッセージ出す
			if atomicgo.PrintError("Failed add Role", err) {
				//embedのData作成
				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Description: "えらー : 追加できませんでした",
					Color:       1000,
				}
				//送信
				_, err := discord.ChannelMessageSendEmbed(rData.ChannelID, embed)
				atomicgo.PrintError("Failed send add role error Embed", err)
			}
			return
		}
	}
}

//リアクション削除でCall
func onMessageReactionRemove(discord *discordgo.Session, reaction *discordgo.MessageReactionRemove) {
	rData := atomicgo.ReactionRemoveViewAndEdit(discord, reaction)

	//embedがあるか確認
	if rData.MessageData.Embeds == nil {
		return
	}

	//Roleのやつか確認
	for _, embed := range rData.MessageData.Embeds {
		footerData := embed.Footer
		if footerData == nil || !strings.Contains(embed.Footer.Text, "RoleContoler") {
			return
		}
	}

	//複配列をstringに変換
	text := ""
	for _, embed := range rData.MessageData.Embeds {
		text = text + embed.Description
	}

	//stringを配列にして1個ずつ処理
	for _, embed := range strings.Split(text, "\n") {
		//ロール追加
		if strings.HasPrefix(embed, rData.Emoji) {
			replace := regexp.MustCompile(`[^0-9]`)
			roleID := replace.ReplaceAllString(embed, "")
			err := discord.GuildMemberRoleRemove(rData.GuildID, rData.UserID, roleID)
			//失敗時メッセージ出す
			if atomicgo.PrintError("Failed remove Role", err) {
				//embedのData作成
				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Description: "えらー : 削除できませんでした",
					Color:       1000,
				}
				//送信
				_, err := discord.ChannelMessageSendEmbed(rData.ChannelID, embed)
				atomicgo.PrintError("Failed send remove role error Embed", err)
			}
			return
		}
	}
}
