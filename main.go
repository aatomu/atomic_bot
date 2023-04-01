package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"time"

	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/atomu21263/atomicgo"
	"github.com/atomu21263/slashlib"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/language"
)

type Sessions struct {
	save   sync.Mutex
	guilds []*SessionData
}

type SessionData struct {
	guildID    string
	channelID  string
	vcsession  *discordgo.VoiceConnection
	lead       sync.Mutex
	enableBot  bool
	mutedUsers []string
	updateInfo bool
}

type UserSetting struct {
	Lang  string  `json:"language"`
	Speed float64 `json:"speed"`
	Pitch float64 `json:"pitch"`
}

var (
	//変数定義
	clientID = ""
	token    = flag.String("token", "", "bot token")
	sessions Sessions
	dummy    = UserSetting{
		Lang:  "auto",
		Speed: 1.5,
		Pitch: 1.1,
	}
)

func main() {
	//flag入手
	flag.Parse()
	fmt.Println("token        :", *token)

	//bot起動準備
	discord := atomicgo.DiscordBotSetup(*token)

	//eventトリガー設定
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onInteractionCreate)
	discord.AddHandler(onVoiceStateUpdate)

	//起動
	atomicgo.DiscordBotStart(discord)
	defer func() {
		for _, session := range sessions.guilds {
			atomicgo.SendEmbed(discord, session.channelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "__Infomation__",
				Description: "Sorry. Bot will Shutdown. Will be try later.",
				Color:       0x00008f,
			})
		}
		atomicgo.DiscordBotEnd(discord)
	}()
	//起動メッセージ表示
	fmt.Println("Listening...")

	//bot停止対策
	atomicgo.StopWait()
}

// BOTの準備が終わったときにCall
func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID

	// コマンドの追加
	new(slashlib.Command).
		//TTS
		AddCommand("join", "VoiceChatに接続します").
		AddCommand("leave", "VoiceChatから切断します").
		AddCommand("get", "読み上げ設定を表示します").
		AddCommand("set", "読み上げ設定を変更します").
		AddOption(slashlib.TypeFloat, "speed", "読み上げ速度を設定", false, 0.5, 5).
		AddOption(slashlib.TypeFloat, "pitch", "声の高さを設定", false, 0.5, 1.5).
		AddOption(slashlib.TypeString, "lang", "読み上げ言語を設定", false, 0, 0).
		AddCommand("dic", "辞書を設定します").
		AddOption(slashlib.TypeString, "from", "置換元", true, 0, 0).
		AddOption(slashlib.TypeString, "to", "置換先", true, 0, 0).
		AddCommand("read", "Botメッセージを読み上げるか変更します").
		AddCommand("mute", "指定されたユーザーメッセージの読み上げを変更します").
		AddOption(slashlib.TypeUser, "user", "読み上げするかを変更するユーザー", true, 0, 0).
		AddCommand("update", "参加,退出を通知します").
		//その他
		AddCommand("poll", "投票を作成します").
		AddOption(slashlib.TypeString, "title", "投票のタイトル", true, 0, 0).
		AddOption(slashlib.TypeString, "choice_1", "選択肢 1", true, 0, 0).
		AddOption(slashlib.TypeString, "choice_2", "選択肢 2", true, 0, 0).
		AddOption(slashlib.TypeString, "choice_3", "選択肢 3", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_4", "選択肢 4", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_5", "選択肢 5", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_6", "選択肢 6", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_7", "選択肢 7", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_8", "選択肢 8", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_9", "選択肢 9", false, 0, 0).
		AddOption(slashlib.TypeString, "choice_10", "選択肢 10", false, 0, 0).
		//AddCommand("role", "ロール管理メニューを生成します").
		CommandCreate(discord, "")
}

// メッセージが送られたときにCall
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	// state update
	joinedGuilds := len(discord.State.Guilds)
	joinedVC := len(sessions.guilds)
	VC := ""
	if joinedVC != 0 {
		VC = fmt.Sprintf(" %d鯖でお話し中", joinedVC)
	}
	atomicgo.BotStateUpdate(discord, fmt.Sprintf("/join | %d鯖で稼働中 %s", joinedGuilds, VC), 0)

	mData := atomicgo.MessageParse(discord, m)
	log.Println(mData.FormatText)

	// 読み上げ無し のチェック
	if strings.HasPrefix(m.Content, ";") {
		return
	}

	// debug
	if atomicgo.StringCheck(mData.Message, "^!debug") && mData.UserID == "701336137012215818" {
		// セッション処理
		if atomicgo.StringCheck(mData.Message, "[0-9]$") {
			guildID := atomicgo.StringReplace(mData.Message, "", `^a debug\s*`)
			log.Println("Deleting SessionItem : " + guildID)
			sessions.Delete(guildID)
			return
		}

		// ユーザー一覧
		VCdata := map[string][]string{}
		for _, guild := range discord.State.Guilds {
			for _, vs := range guild.VoiceStates {
				user, err := discord.User(vs.UserID)
				if err != nil {
					continue
				}
				VCdata[vs.GuildID] = append(VCdata[vs.GuildID], user.String())
			}
		}

		// 表示
		for _, session := range sessions.guilds {
			guild, err := discord.Guild(session.guildID)
			if atomicgo.PrintError("Failed Get GuildData by GuildID", err) {
				continue
			}

			channel, err := discord.Channel(session.channelID)
			if atomicgo.PrintError("Failed Get ChannelData by ChannelID", err) {
				continue
			}

			atomicgo.SendEmbed(discord, mData.ChannelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       fmt.Sprintf("Guild:%s(%s)\nChannel:%s(%s)", guild.Name, session.guildID, channel.Name, session.channelID),
				Description: fmt.Sprintf("Members:```\n%s```", VCdata[guild.ID]),
				Color:       0xff00ff,
			})
		}
		return
	}

	//読み上げ
	session := sessions.Get(mData.GuildID)
	isMuted := false
	if session != nil {
		for _, mutedUserID := range session.mutedUsers {
			if mutedUserID == mData.UserID {
				isMuted = true
			}
		}
		if session.IsJoined() && !isMuted && session.channelID == mData.ChannelID && !(m.Author.Bot && !session.enableBot) {
			session.Speech(mData.UserID, mData.Message)
			return
		}
	}
}

// InteractionCreate
func onInteractionCreate(discord *discordgo.Session, iData *discordgo.InteractionCreate) {
	// 表示&処理しやすく
	i := slashlib.InteractionViewAndEdit(discord, iData)

	// slashじゃない場合return
	if i.Check != slashlib.SlashCommand {
		return
	}

	// response用データ
	res := slashlib.InteractionResponse{
		Discord:     discord,
		Interaction: iData.Interaction,
	}

	session := sessions.Get(i.GuildID)
	// 分岐
	switch i.Command.Name {
	//TTS
	case "join":
		res.Thinking(false)

		if session.IsJoined() {
			Failed(res, "VoiceChat にすでに接続しています")
			return
		}

		vcSession, err := atomicgo.JoinUserVCchannel(discord, i.UserID, false, true)
		if atomicgo.PrintError("Failed Join VoiceChat", err) {
			Failed(res, "ユーザーが VoiceChatに接続していない\nもしくは権限が不足しています")
			return
		}

		session := &SessionData{
			guildID:   i.GuildID,
			channelID: i.ChannelID,
			vcsession: vcSession,
			lead:      sync.Mutex{},
		}

		sessions.Add(session)

		session.Speech("BOT", "おはー")
		Success(res, "ハロー!")

		go func(guildID string, channelName string) {
			ticker := time.NewTicker(3 * time.Minute)
			defer ticker.Stop()
			for {
				<-ticker.C
				session := sessions.Get(guildID)
				if session == nil {
					log.Println("Connection Close", channelName)
					break
				}
				var end chan bool
				err := atomicgo.PlayAudioFile(10.00, 1.00, session.vcsession, "http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%20&tl=ja", false, end)
				if err != nil {
					log.Println("Ping Send Failed", channelName, "Err:", err)
				}
			}
			log.Println("Auto Ping Return", channelName)
		}(i.GuildID, i.ChannelName)

		return

	case "leave":
		res.Thinking(false)

		if !session.IsJoined() {
			Failed(res, "VoiceChat に接続していません")
			return
		}

		session.Speech("BOT", "さいなら")
		Success(res, "グッバイ!")
		time.Sleep(1 * time.Second)
		session.vcsession.Disconnect()

		sessions.Delete(i.GuildID)
		return

	case "get":
		res.Thinking(false)

		result, err := userConfig(i.UserID, UserSetting{})
		if atomicgo.PrintError("Failed Get Config", err) {
			Failed(res, "データのアクセスに失敗しました。")
			return
		}

		res.Follow(&discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       fmt.Sprintf("@%s's Speech Config", i.UserName),
					Description: fmt.Sprintf("```\nLang  : %4s\nSpeed : %3.2f\nPitch : %3.2f```", result.Lang, result.Speed, result.Pitch),
				},
			},
		})
		return

	case "set":
		res.Thinking(false)

		// 保存
		result, err := userConfig(i.UserID, UserSetting{})
		if atomicgo.PrintError("Failed Get Config", err) {
			Failed(res, "読み上げ設定を読み込めませんでした")
			return
		}

		// チェック
		if newSpeed, ok := i.CommandOptions["speed"]; ok {
			result.Speed = newSpeed.FloatValue()
		}
		if newPitch, ok := i.CommandOptions["pitch"]; ok {
			result.Pitch = newPitch.FloatValue()
		}
		if newLang, ok := i.CommandOptions["lang"]; ok {
			result.Lang = newLang.StringValue()
			// 言語チェック
			_, err := language.Parse(result.Lang)
			if result.Lang != "auto" && err != nil {
				Failed(res, "不明な言語です\n\"auto\"もしくは言語コードのみ使用可能です")
				return
			}
		}

		_, err = userConfig(i.UserID, result)
		if atomicgo.PrintError("Failed Write Config", err) {
			Failed(res, "保存に失敗しました")
		}
		Success(res, "読み上げ設定を変更しました")
		return

	case "dic":
		res.Thinking(false)

		//ファイルの指定
		fileName := "./dic/" + i.GuildID + ".txt"
		//dicがあるか確認
		if !CheckDic(i.GuildID) {
			Failed(res, "辞書の読み込みに失敗しました")
			return
		}

		textByte, _ := atomicgo.ReadFile(fileName)
		dic := string(textByte)

		//textをfrom toに
		from := i.CommandOptions["from"].StringValue()
		to := i.CommandOptions["to"].StringValue()

		// 禁止文字チェック
		if strings.Contains(from, ",") || strings.Contains(to, ",") {
			Failed(res, "使用できない文字が含まれています")
			return
		}

		//確認
		if strings.Contains(dic, from+",") {
			dic = atomicgo.StringReplace(dic, "", "\n"+from+",.*")
		}
		dic = dic + from + "," + to + "\n"

		//書き込み
		ok := atomicgo.WriteFileFlash(fileName, []byte(dic), 0777)
		if !ok {
			Failed(res, "辞書の書き込みに失敗しました")
			return
		}

		Success(res, "辞書を保存しました\n\""+from+"\" => \""+to+"\"")
		return

	case "read":
		res.Thinking(false)

		// VC接続中かチェック
		if !session.IsJoined() {
			Failed(res, "VoiceChat に接続していません")
			return
		}

		session.enableBot = !session.enableBot

		Success(res, fmt.Sprintf("Botメッセージの読み上げを %t に変更しました", session.enableBot))
		return

	case "mute":
		res.Thinking(false)

		// VC接続中かチェック
		if !session.IsJoined() {
			Failed(res, "VoiceChat に接続していません")
			return
		}

		user := i.CommandOptions["user"].UserValue(discord)
		if user == nil {
			Failed(res, "Unknown User")
			return
		}
		toMute := true
		for _, userID := range session.mutedUsers {
			if userID == user.ID {
				toMute = false
			}
		}
		if toMute {
			session.mutedUsers = append(session.mutedUsers, user.ID)
		} else {
			var newMutedUsers []string
			for _, userID := range session.mutedUsers {
				if userID == user.ID {
					continue
				}
				newMutedUsers = append(newMutedUsers, userID)
			}
			session.mutedUsers = newMutedUsers
		}
		Success(res, fmt.Sprintf("%s のメッセージの読み上げを %t に変更しました", user.String(), !toMute))
		return

	case "update":
		res.Thinking(false)

		// VC接続中かチェック
		if !session.IsJoined() {
			Failed(res, "VoiceChat に接続していません")
			return
		}

		session.updateInfo = !session.updateInfo

		Success(res, fmt.Sprintf("ボイスチャットの参加/退出の通知を %t に変更しました", session.updateInfo))
		return

		//その他
	case "poll":
		res.Thinking(false)

		title := i.CommandOptions["title"].StringValue()
		choices := []string{}
		choices = append(choices, i.CommandOptions["choice_1"].StringValue())
		choices = append(choices, i.CommandOptions["choice_2"].StringValue())
		if value, ok := i.CommandOptions["choice_3"]; ok {
			choices = append(choices, value.StringValue())
		}
		if value, ok := i.CommandOptions["choice_4"]; ok {
			choices = append(choices, value.StringValue())
		}
		if value, ok := i.CommandOptions["choice_5"]; ok {
			choices = append(choices, value.StringValue())
		}
		description := ""
		reaction := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}
		for i := 0; i < len(choices); i++ {
			description += fmt.Sprintf("%s : %s\n", reaction[i], choices[i])
		}
		m, err := res.Follow(&discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       title,
					Color:       0x1E90FF,
					Description: description,
				},
			},
		})
		if atomicgo.PrintError("Failed Follow", err) {
			return
		}
		time.Sleep(1 * time.Second)
		for i := 0; i < len(choices); i++ {
			discord.MessageReactionAdd(m.ChannelID, m.ID, reaction[i])
		}
	}
}

func userConfig(userID string, user UserSetting) (result UserSetting, err error) {
	//BOTチェック
	if userID == "BOT" {
		return UserSetting{
			Lang:  "ja",
			Speed: 1.75,
			Pitch: 1,
		}, nil
	}

	//ファイルパスの指定
	fileName := "./user_config.json"

	if !atomicgo.CheckFile(fileName) {
		if !atomicgo.CreateFile(fileName) {
			return dummy, fmt.Errorf("failed Create Config File")
		}
	}

	bytes, ok := atomicgo.ReadFile(fileName)
	if !ok {
		return dummy, fmt.Errorf("failed Read Config File")
	}

	Users := map[string]UserSetting{}
	if string(bytes) != "" {
		err = json.Unmarshal(bytes, &Users)
		atomicgo.PrintError("failed UnMarshal UserConfig", err)
	}

	// チェック用
	nilUserSetting := UserSetting{}
	//上書き もしくはデータ作成
	// result が  nil とき 書き込み
	if _, ok := Users[userID]; !ok {
		result = dummy
		if user == nilUserSetting {
			return
		}
	}
	if config, ok := Users[userID]; ok && user == nilUserSetting {
		return config, nil
	}

	// 書き込み
	if user != nilUserSetting {
		//lang
		if user.Lang != "" {
			result.Lang = user.Lang
		}
		//speed
		if user.Speed != 0.0 {
			result.Speed = user.Speed
		}
		//pitch
		if user.Pitch != 0 {
			result.Pitch = user.Pitch
		}
		//最後に書き込むテキストを追加(Write==trueの時)
		Users[userID] = result
		bytes, err = json.MarshalIndent(&Users, "", "  ")
		fmt.Println(string(bytes))
		if err != nil {
			return dummy, fmt.Errorf("failed Marshal UserConfig")
		}
		//書き込み
		atomicgo.WriteFileFlash(fileName, bytes, 0755)
		log.Println("User userConfig Writed")
	}
	return
}

// VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	vData := atomicgo.VoiceStateParse(discord, v)
	if !vData.StatusUpdate.ChannelJoin {
		return
	}
	log.Println(vData.FormatText)

	//セッションがあるか確認
	session := sessions.Get(v.GuildID)
	if session == nil {
		return
	}
	vcChannelID := session.vcsession.ChannelID

	// ボイスチャンネルに誰かいるか
	isLeave := true
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if vcChannelID == vs.ChannelID && vs.UserID != clientID {
				isLeave = false
				break
			}
		}
	}

	if isLeave {
		// ボイスチャンネルに誰もいなかったら Disconnect する
		session.vcsession.Disconnect()
		sessions.Delete(v.GuildID)
	} else {
		// でなければ通知?
		if !session.updateInfo {
			return
		}
		if vData.Status.ChannelJoin {
			session.Speech("BOT", fmt.Sprintf("%s join the voice", vData.UserData.Username))
		} else { // 今 VCchannelIDがない
			session.Speech("BOT", fmt.Sprintf("%s left the voice", vData.UserData.Username))
		}
	}
}

// Get Session
func (s *Sessions) Get(guildID string) *SessionData {
	for _, session := range s.guilds {
		if session.guildID != guildID {
			continue
		}
		return session
	}
	return nil
}

// Add Session
func (s *Sessions) Add(newSession *SessionData) {
	s.save.Lock()
	defer s.save.Unlock()
	s.guilds = append(s.guilds, newSession)
}

// Delete Session
func (s *Sessions) Delete(guildID string) {
	s.save.Lock()
	defer s.save.Unlock()
	var newSessions []*SessionData
	for _, session := range s.guilds {
		if session.guildID == guildID {
			if session.vcsession != nil {
				session.vcsession.Disconnect()
			}
			continue
		}
		newSessions = append(newSessions, session)
	}
	s.guilds = newSessions
}

// Is Joined Session
func (session *SessionData) IsJoined() bool {
	return session != nil
}

// Speech in Session
func (session *SessionData) Speech(userID string, text string) {
	if CheckDic(session.guildID) {
		data, _ := os.Open("./dic/" + session.guildID + ".txt")
		defer data.Close()

		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			line := scanner.Text()
			words := strings.Split(line, ",")
			text = strings.ReplaceAll(text, words[0], words[1])
		}
	}

	if regexp.MustCompile(`<a:|<:|<@|<#|<@&|http|` + "```").MatchString(text) {
		text = "すーきっぷ"
	}

	//! ? { } < >を読み上げない
	replace := regexp.MustCompile(`!|\?|{|}|<|>`)
	text = replace.ReplaceAllString(text, "")

	settingData, err := userConfig(userID, UserSetting{})
	atomicgo.PrintError("Failed func userConfig()", err)

	if settingData.Lang == "auto" {
		settingData.Lang = "ja"
		if regexp.MustCompile(`^[a-zA-Z0-9\s.,]+$`).MatchString(text) {
			settingData.Lang = "en"
		}
	}

	//隠れてるところを読み上げない
	if strings.Contains(text, "||") {
		replace := regexp.MustCompile(`(?s)\|\|.*\|\|`)
		text = replace.ReplaceAllString(text, "ピーーーー")
	}

	// //改行停止
	// if strings.Contains(text, "\n") {
	// 	replace := regexp.MustCompile(`\n.*`)
	// 	text = replace.ReplaceAllString(text, "")
	// }

	//text cut
	read := atomicgo.StringCut(text, 100)

	//読み上げ待機
	session.lead.Lock()
	defer session.lead.Unlock()

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%s&tl=%s", url.QueryEscape(read), settingData.Lang)
	var end chan bool
	err = atomicgo.PlayAudioFile(settingData.Speed, settingData.Pitch, session.vcsession, voiceURL, false, end)
	atomicgo.PrintError("Failed play Audio \""+read+"\" ", err)
}

// Command Failed Message
func Failed(res slashlib.InteractionResponse, description string) {
	_, err := res.Follow(&discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{
			{
				Title:       "Command Failed",
				Color:       0xDC143C,
				Description: description,
			},
		},
	})
	atomicgo.PrintError("Failed send response", err)
}

// Command Success Message
func Success(res slashlib.InteractionResponse, description string) {
	_, err := res.Follow(&discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{
			{
				Title:       "Command Success",
				Color:       0x1E90FF,
				Description: description,
			},
		},
	})
	atomicgo.PrintError("Failed send response", err)
}

func CheckDic(guildID string) (ok bool) {
	// dic.txtがあるか
	if atomicgo.CheckFile("./dic/" + guildID + ".txt") {
		return true
	}

	//フォルダがあるか確認
	if !atomicgo.CheckFile("./dic") {
		//フォルダがなかったら作成
		success := atomicgo.CreateDir("./dic", 0755)
		if !success {
			return false
		}
	}

	//ファイル作成
	success := atomicgo.WriteFileFlash("./dic/"+guildID+".txt", []byte{}, 0777)
	if !success {
		atomicgo.PrintError("Failed create dictionary", fmt.Errorf("permission denied?"))
		return false
	}
	return true
}
