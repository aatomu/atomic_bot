package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"strings"

	"github.com/aatomu/aatomlib/disgord"
	"github.com/aatomu/aatomlib/utils"
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
	vc         *discordgo.VoiceConnection
	lead       sync.Mutex
	updateInfo bool
}

type UserSetting struct {
	Lang  string  `json:"language"`
	Speed float64 `json:"speed"`
	Pitch float64 `json:"pitch"`
}

var (
	//変数定義
	clientID              = ""
	token                 = flag.String("token", "", "bot token")
	sessions              Sessions
	isVcSessionUpdateLock = false
	dummy                 = UserSetting{
		Lang:  "auto",
		Speed: 1.5,
		Pitch: 1.1,
	}
	embedColor = 0x1E90FF
	logger     = utils.LoggerHandler{Level: utils.Warn}
)

func main() {
	flag.Parse()
	fmt.Println("token        :", *token)

	// Initialize bot
	discord, err := discordgo.New("Bot " + *token)
	if err != nil {
		logger.Error("Failed bot initialize", err)
		return
	}

	// Set event handlers
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onInteractionCreate)
	discord.AddHandler(onVoiceStateUpdate)

	// Connect to Discord
	discord.Open()
	defer func() {
		for _, session := range sessions.guilds {
			discord.ChannelMessageSendEmbed(session.channelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "__Information__",
				Description: "Sorry. Bot will Shutdown. Will be try later.",
				Color:       embedColor,
			})
		}
		discord.Close()
	}()

	<-utils.BreakSignal()
}

func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	logger.Info("Discord bot on ready")
	clientID = discord.State.User.ID

	// Add slash command
	var minSpeed float64 = 0.5
	var minPitch float64 = 0.5
	disgord.InteractionCommandCreate(discord, "", []*discordgo.ApplicationCommand{
		// Voice commands
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "join",
			Description:              "VoiceChatに接続します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "leave",
			Description:              "VoiceChatから切断します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "get",
			Description:              "読み上げ設定を表示します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "set",
			Description:              "読み上げ設定を変更します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionNumber, Name: "speed", Description: "読み上げ速度を設定", MinValue: &minSpeed, MaxValue: 5},
				{Type: discordgo.ApplicationCommandOptionNumber, Name: "pitch", Description: "声の高さを設定", MinValue: &minPitch, MaxValue: 1.5},
				{Type: discordgo.ApplicationCommandOptionString, Name: "lang", Description: "読み上げ言語を設定"},
			},
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "dic",
			Description:              "辞書を設定します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "from", Description: "置換元", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "to", Description: "置換先", Required: true},
			},
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "update",
			Description:              "参加,退出を通知します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
		},
	})
}

func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	// bot status update
	joinedGuilds := len(discord.State.Guilds)
	joinedVC := len(sessions.guilds)
	discord.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: string(discordgo.StatusOnline),
		Activities: []*discordgo.Activity{
			{
				Name:  "i'm a bot",
				Type:  discordgo.ActivityTypeListening,
				State: fmt.Sprintf("Working on %d servers (Speech for %d servers)", joinedGuilds, joinedVC),
			},
		},
	})

	// Update voice session
	go func() {
		if isVcSessionUpdateLock {
			return
		}

		// Update check
		isVcSessionUpdateLock = true
		defer func() {
			time.Sleep(1 * time.Minute)
			isVcSessionUpdateLock = false
		}()

		for i := range sessions.guilds {
			go func(n int) {
				session := sessions.guilds[n]
				session.lead.Lock()
				defer session.lead.Unlock()
				session.vc = discord.VoiceConnections[session.guildID]
			}(i)
		}
	}()

	mData := disgord.MessageParse(discord, m.Message)
	if mData.User.Bot {
		return
	}
	logger.Info(toJson(m))

	// Check reading skip
	if strings.HasPrefix(m.Content, ";") || mData.Message == nil {
		return
	}

	// debug
	if mData.User.ID == "701336137012215818" {
		switch {
		case utils.RegMatch(mData.Message.Content, "^!debug"):
			// Session delete
			if utils.RegMatch(mData.Message.Content, "[0-9]$") {
				guildID := utils.RegReplace(mData.Message.Content, "", `^!debug\s*`)
				logger.Info("Deleting SessionItem : " + guildID)
				sessions.Delete(guildID)
				return
			}

			// Voice channel user list
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

			// Return voice connection information
			for _, session := range sessions.guilds {
				guild, err := discord.Guild(session.guildID)
				if utils.PrintError("Failed Get GuildData by GuildID", err) {
					continue
				}

				channel, err := discord.Channel(session.channelID)
				if utils.PrintError("Failed Get ChannelData by ChannelID", err) {
					continue
				}

				embed, err := discord.ChannelMessageSendEmbed(mData.ChannelID, &discordgo.MessageEmbed{
					Type:        "rich",
					Title:       fmt.Sprintf("Guild:%s(%s)\nChannel:%s(%s)", guild.Name, session.guildID, channel.Name, session.channelID),
					Description: fmt.Sprintf("Members:```\n%s```", VCdata[guild.ID]),
					Color:       embedColor,
				})
				if err == nil {
					go func() {
						time.Sleep(30 * time.Second)
						err := discord.ChannelMessageDelete(mData.ChannelID, embed.ID)
						utils.PrintError("failed delete debug message", err)
					}()
				}
			}
			if len(sessions.guilds) == 0 {
				embed, err := discord.ChannelMessageSendEmbed(mData.ChannelID, &discordgo.MessageEmbed{
					Type:  "rich",
					Title: "Session Not Found",
					Color: embedColor,
				})
				if err == nil {
					go func() {
						time.Sleep(30 * time.Second)
						err := discord.ChannelMessageDelete(mData.ChannelID, embed.ID)
						utils.PrintError("failed delete debug message", err)
					}()
				}
			}
			return
		}
	}

	//読み上げ
	session := sessions.Get(mData.GuildID)
	if session != nil {
		if session.IsJoined() && session.channelID == mData.ChannelID {
			session.Speech(mData.User.ID, mData.Message.Content)
			return
		}
	}
}

// InteractionCreate
func onInteractionCreate(discord *discordgo.Session, i *discordgo.InteractionCreate) {
	// 表示&処理しやすく
	iData := disgord.InteractionParse(discord, i.Interaction)
	logger.Info(toJson(i))

	// response用データ
	res := disgord.NewInteractionResponse(discord, i.Interaction)

	// 分岐
	switch iData.Command.Name {
	//TTS
	case "join":
		res.Thinking(false)

		session := sessions.Get(iData.GuildID)
		if session.IsJoined() {
			sessions.Failed(res, "VoiceChat にすでに接続しています")
			return
		}

		session.JoinVoice(res, discord, iData.GuildID, iData.ChannelID, iData.User.ID)
		return

	case "leave":
		res.Thinking(false)

		session := sessions.Get(iData.GuildID)
		if !session.IsJoined() {
			sessions.Failed(res, "VoiceChat に接続していません")
			return
		}
		session.LeaveVoice(res)

	case "get":
		res.Thinking(false)

		result, err := sessions.Config(iData.User.ID, UserSetting{})
		if utils.PrintError("Failed Get Config", err) {
			sessions.Failed(res, "データのアクセスに失敗しました。")
			return
		}

		res.Follow(&discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       fmt.Sprintf("@%s's Speech Config", iData.User.Username),
					Description: fmt.Sprintf("```\nLang  : %4s\nSpeed : %3.2f\nPitch : %3.2f```", result.Lang, result.Speed, result.Pitch),
				},
			},
		})
		return

	case "set":
		res.Thinking(false)

		sessions.UpdateConfig(res, iData)
		return

	case "dic":
		res.Thinking(false)

		session := sessions.Get(iData.GuildID)
		if !session.IsJoined() {
			sessions.Failed(res, "VoiceChat に接続していません")
			return
		}

		session.Dictionary(res, iData)
		return

	case "update":
		res.Thinking(false)

		session := sessions.Get(iData.GuildID)
		if !session.IsJoined() {
			sessions.Failed(res, "VoiceChat に接続していません")
			return
		}

		session.ToggleUpdate(res)
		return
	}
}

// VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	vData := disgord.VoiceStateParse(discord, v)
	if !vData.UpdateStatus.ChannelJoin {
		return
	}
	logger.Info(toJson(v))

	//セッションがあるか確認
	session := sessions.Get(v.GuildID)
	if session == nil {
		return
	}
	session.AutoLeave(discord, vData.Status.ChannelJoin, vData.User.Username)
}

func Pinter(n int64) *int64 {
	return &n
}

func toJson(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Sessions) Get(guildID string) *SessionData {
	for _, session := range s.guilds {
		if session.guildID != guildID {
			continue
		}
		return session
	}
	return nil
}

func (s *SessionData) IsJoined() bool {
	return s != nil
}

func (s *Sessions) Add(newSession *SessionData) {
	s.save.Lock()
	defer s.save.Unlock()
	s.guilds = append(s.guilds, newSession)
}

func (s *Sessions) Delete(guildID string) {
	s.save.Lock()
	defer s.save.Unlock()
	var newSessions []*SessionData
	for _, session := range s.guilds {
		if session.guildID == guildID {
			if session.vc != nil {
				session.vc.Disconnect()
			}
			continue
		}
		newSessions = append(newSessions, session)
	}
	s.guilds = newSessions
}

func (s *SessionData) JoinVoice(res *disgord.InteractionResponse, discord *discordgo.Session, guildID, channelID, userID string) {
	vcSession, err := disgord.JoinUserVCchannel(discord, userID, false, true)
	if utils.PrintError("Failed Join VoiceChat", err) {
		sessions.Failed(res, "ユーザーが VoiceChatに接続していない\nもしくは権限が不足しています")
		return
	}

	// vcSession.LogLevel = discordgo.LogDebug

	session := &SessionData{
		guildID:   guildID,
		channelID: channelID,
		vc:        vcSession,
		lead:      sync.Mutex{},
	}

	sessions.Add(session)
	go func() {
		for {
			if sessions.Get(guildID) == nil {
				return
			}
			session.lead.Lock()
			session.vc.Speaking(false)
			session.lead.Unlock()
			time.Sleep(10 * time.Second)
		}
	}()

	session.Speech("BOT", "おはー")
	sessions.Success(res, "ハロー!")
}

func (s *SessionData) LeaveVoice(res *disgord.InteractionResponse) {
	s.Speech("BOT", "さいなら")
	sessions.Success(res, "グッバイ!")
	time.Sleep(1 * time.Second)
	s.vc.Disconnect()

	sessions.Delete(s.guildID)
}

func (s *SessionData) AutoLeave(discord *discordgo.Session, isJoin bool, userName string) {
	checkVcChannelID := s.vc.ChannelID

	// ボイスチャンネルに誰かいるか
	isLeave := true
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if checkVcChannelID == vs.ChannelID && vs.UserID != clientID {
				isLeave = false
				break
			}
		}
	}

	if isLeave {
		// ボイスチャンネルに誰もいなかったら Disconnect する
		s.vc.Disconnect()
		sessions.Delete(s.guildID)
	} else {
		// でなければ通知?
		if !s.updateInfo {
			return
		}
		if isJoin {
			s.Speech("BOT", fmt.Sprintf("%s join the voice", userName))
		} else {
			s.Speech("BOT", fmt.Sprintf("%s left the voice", userName))
		}
	}
}

func (session *SessionData) Speech(userID string, text string) {
	if session.CheckDic() {
		data, _ := os.Open(filepath.Join(".", "dic", session.guildID+".txt"))
		defer data.Close()

		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			line := scanner.Text()
			words := strings.Split(line, ",")
			text = strings.ReplaceAll(text, words[0], words[1])
		}
	}

	// Special Character
	text = regexp.MustCompile(`<a?:[A-Za-z0-9_]+?:[0-9]+?>`).ReplaceAllString(text, "えもじ") // custom Emoji
	text = regexp.MustCompile(`<@[0-9]+?>`).ReplaceAllString(text, "あっと ゆーざー")             // newConfig mention
	text = regexp.MustCompile(`<@&[0-9]+?>`).ReplaceAllString(text, "あっと ろーる")             // newConfig mention
	text = regexp.MustCompile(`<#[0-9]+?>`).ReplaceAllString(text, "あっと ちゃんねる")            // channel
	text = regexp.MustCompile(`https?:.+`).ReplaceAllString(text, "ゆーあーるえる すーきっぷ")         // URL
	text = regexp.MustCompile(`(?s)\|\|.*\|\|`).ReplaceAllString(text, "ひみつ")              // hidden word
	// Word Decoration 3
	text = regexp.MustCompile(`>>> `).ReplaceAllString(text, "")                      // quote
	text = regexp.MustCompile("(?s)```.*```").ReplaceAllString(text, "こーどぶろっく すーきっぷ") // codeblock
	// Word Decoration 2
	text = regexp.MustCompile(`~~(.+)~~`).ReplaceAllString(text, "$1")     // strike through
	text = regexp.MustCompile(`__(.+)__`).ReplaceAllString(text, "$1")     // underlined
	text = regexp.MustCompile(`\*\*(.+)\*\*`).ReplaceAllString(text, "$1") // bold
	// Word Decoration 1
	text = regexp.MustCompile(`> `).ReplaceAllString(text, "")         // 1line quote
	text = regexp.MustCompile("`(.+)`").ReplaceAllString(text, "$1")   // code
	text = regexp.MustCompile(`_(.+)_`).ReplaceAllString(text, "$1")   // italic
	text = regexp.MustCompile(`\*(.+)\*`).ReplaceAllString(text, "$1") // bold
	// Delete black Newline
	text = regexp.MustCompile(`^\n+`).ReplaceAllString(text, "")
	// Delete More Newline
	if strings.Count(text, "\n") > 5 {
		str := strings.Split(text, "\n")
		text = strings.Join(str[:5], "\n")
		text += "以下略"
	}
	//text cut
	read := utils.StrCut(text, "以下略", 100)

	settingData, err := sessions.Config(userID, UserSetting{})
	utils.PrintError("Failed func userConfig()", err)

	if settingData.Lang == "auto" {
		settingData.Lang = "ja"
		if regexp.MustCompile(`^[a-zA-Z0-9\s.,]+$`).MatchString(text) {
			settingData.Lang = "en"
		}
	}

	//読み上げ待機
	session.lead.Lock()
	defer session.lead.Unlock()

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%s&tl=%s", url.QueryEscape(read), settingData.Lang)
	err = disgord.PlayAudioFile(session.vc, voiceURL, settingData.Speed, settingData.Pitch, 1, false, make(<-chan bool))
	utils.PrintError("Failed play Audio \""+read+"\" ", err)
}

func (s *SessionData) Dictionary(res *disgord.InteractionResponse, i disgord.InteractionData) {
	//ファイルの指定
	fileName := filepath.Join(".", "dic", s.guildID+".txt")
	//dicがあるか確認
	if !s.CheckDic() {
		sessions.Failed(res, "辞書の読み込みに失敗しました")
		return
	}

	textByte, _ := os.ReadFile(fileName)
	dic := string(textByte)

	//textをfrom toに
	from := i.CommandOptions["from"].StringValue()
	to := i.CommandOptions["to"].StringValue()

	// 禁止文字チェック
	if strings.Contains(from, ",") || strings.Contains(to, ",") {
		sessions.Failed(res, "使用できない文字が含まれています")
		return
	}

	//確認
	if strings.Contains(dic, from+",") {
		dic = utils.RegReplace(dic, "", "\n"+from+",.*")
	}
	dic = dic + from + "," + to + "\n"

	//書き込み
	err := os.WriteFile(fileName, []byte(dic), 0755)
	if utils.PrintError("Config Update Failed", err) {
		sessions.Failed(res, "辞書の書き込みに失敗しました")
		return
	}

	sessions.Success(res, "辞書を保存しました\n\""+from+"\" => \""+to+"\"")
}

func (s *SessionData) ToggleUpdate(res *disgord.InteractionResponse) {
	s.updateInfo = !s.updateInfo

	sessions.Success(res, fmt.Sprintf("ボイスチャットの参加/退出の通知を %t に変更しました", s.updateInfo))
}

func (s *SessionData) CheckDic() (ok bool) {
	// dic.txtがあるか
	_, err := os.Stat(filepath.Join(".", "dic", s.guildID+".txt"))
	if err == nil {
		return true
	}

	//フォルダがあるか確認
	_, err = os.Stat(filepath.Join(".", "dic"))
	if os.IsNotExist(err) {
		//フォルダがなかったら作成
		err := os.Mkdir(filepath.Join(".", "dic"), 0755)
		if utils.PrintError("Failed Create Dic", err) {
			return false
		}
	}

	//ファイル作成
	f, err := os.Create(filepath.Join(".", "dic", s.guildID+".txt"))
	f.Close()
	return !utils.PrintError("Failed create dictionary", err)
}

func (s *Sessions) Config(userID string, newConfig UserSetting) (result UserSetting, err error) {
	//BOTチェック
	if userID == "BOT" {
		return UserSetting{
			Lang:  "ja",
			Speed: 1.75,
			Pitch: 1,
		}, nil
	}

	//ファイルパスの指定
	fileName := filepath.Join(".", "user_config.json")

	_, err = os.Stat(fileName)
	if os.IsNotExist(err) {
		f, err := os.Create(fileName)
		f.Close()
		if err != nil {
			return dummy, fmt.Errorf("failed Create Config File")
		}
	}

	bytes, err := os.ReadFile(fileName)
	if err != nil {
		return dummy, fmt.Errorf("failed Read Config File")
	}

	Users := map[string]UserSetting{}
	if string(bytes) != "" {
		err = json.Unmarshal(bytes, &Users)
		utils.PrintError("failed UnMarshal UserConfig", err)
	}

	// チェック用
	nilUserSetting := UserSetting{}
	//上書き もしくはデータ作成
	// result が  nil とき 書き込み
	if _, ok := Users[userID]; !ok {
		result = dummy
		if newConfig == nilUserSetting {
			return
		}
	}
	if config, ok := Users[userID]; ok && newConfig == nilUserSetting {
		return config, nil
	}

	// 書き込み
	if newConfig != nilUserSetting {
		//lang
		if newConfig.Lang != result.Lang {
			result.Lang = newConfig.Lang
		}
		//speed
		if newConfig.Speed != result.Speed {
			result.Speed = newConfig.Speed
		}
		//pitch
		if newConfig.Pitch != result.Pitch {
			result.Pitch = newConfig.Pitch
		}
		//最後に書き込むテキストを追加(Write==trueの時)
		Users[userID] = result
		bytes, err = json.MarshalIndent(&Users, "", "  ")
		if err != nil {
			return dummy, fmt.Errorf("failed Marshal UserConfig")
		}
		//書き込み
		os.WriteFile(fileName, bytes, 0655)
		logger.Info("User user config write")
	}
	return
}

func (s *Sessions) UpdateConfig(res *disgord.InteractionResponse, i disgord.InteractionData) {
	// 読み込み
	result, err := sessions.Config(i.User.ID, UserSetting{})
	if utils.PrintError("Failed Get Config", err) {
		sessions.Failed(res, "読み上げ設定を読み込めませんでした")
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
			s.Failed(res, "不明な言語です\n\"auto\"もしくは言語コードのみ使用可能です")
			return
		}
	}

	_, err = sessions.Config(i.User.ID, result)
	if utils.PrintError("Failed Write Config", err) {
		sessions.Failed(res, "保存に失敗しました")
	}
	sessions.Success(res, "読み上げ設定を変更しました")
}

func (s *Sessions) Failed(res *disgord.InteractionResponse, description string) {
	_, err := res.Follow(&discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{
			{
				Title:       "Command Failed",
				Color:       embedColor,
				Description: description,
			},
		},
	})
	utils.PrintError("Failed send response", err)
}

func (s *Sessions) Success(res *disgord.InteractionResponse, description string) {
	_, err := res.Follow(&discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{
			{
				Title:       "Command Success",
				Color:       embedColor,
				Description: description,
			},
		},
	})
	utils.PrintError("Failed send response", err)
}
