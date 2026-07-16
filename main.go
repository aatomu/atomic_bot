package main

import "C"
import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aatomu/aatomlib/utils"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
)

type Sessions struct {
	save   sync.Mutex
	guilds []*SessionData
}

type SessionData struct {
	guildID    *snowflake.ID
	channelID  snowflake.ID
	vc         *voice.Conn
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
	clientID              snowflake.ID
	owners                = []snowflake.ID{}
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

	client, err := disgo.New(*token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				// gateway.IntentMessageContent,
				// gateway.IntentGuildMessages,
				gateway.IntentGuildVoiceStates,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagGuilds,
				cache.FlagMembers,
				cache.FlagVoiceStates,
			),
		),
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(golibdave.NewSession),
		),
		// add event listeners
		bot.WithEventListenerFunc(onReady),
		bot.WithEventListenerFunc(onMessageCreate),
		bot.WithEventListenerFunc(onInteractionCreate),
		// bot.WithEventListenerFunc(onVoiceStateUpdate),
	)
	if err != nil {
		panic(err)
	}
	// connect to the gateway
	if err = client.OpenGateway(context.TODO()); err != nil {
		panic(err)
	}

	// Connect to Discord
	defer func() {
		for _, session := range sessions.guilds {
			client.Rest.CreateMessage(session.channelID, discord.NewMessageCreate().AddEmbeds(
				discord.Embed{
					Type:        discord.EmbedTypeRich,
					Title:       "__Information__",
					Description: "Sorry. Bot will Shutdown. Will be try later.",
					Color:       embedColor,
				},
			))
		}
	}()

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s
}

func onReady(r *events.Ready) {
	logger.Info("Discord bot on ready")
	clientID = r.Client().ID()

	// Add slash command
	commands := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:                     "join",
			Description:              "VoiceChatに接続します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
		},
		discord.SlashCommandCreate{
			Name:                     "leave",
			Description:              "VoiceChatから切断します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
		},
		discord.SlashCommandCreate{
			Name:                     "get",
			Description:              "読み上げ設定を表示します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
		},
		discord.SlashCommandCreate{
			Name:                     "set",
			Description:              "読み上げ設定を変更します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionFloat{
					Name:        "speed",
					Description: "読み上げ速度を設定",
					MinValue:    omit.Ptr(0.5),
					MaxValue:    omit.Ptr(5.0),
				},
				discord.ApplicationCommandOptionFloat{
					Name:        "pitch",
					Description: "声の高さを設定",
					MinValue:    omit.Ptr(0.5),
					MaxValue:    omit.Ptr(1.5),
				},
				discord.ApplicationCommandOptionString{
					Name:        "lang",
					Description: "読み上げ言語を設定",
				},
			},
		},
		discord.SlashCommandCreate{
			Name:                     "dic",
			Description:              "辞書を設定します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "from",
					Description: "置換元",
					Required:    true,
				},
				discord.ApplicationCommandOptionString{
					Name:        "to",
					Description: "置換先",
					Required:    true,
				},
			},
		},
		discord.SlashCommandCreate{
			Name:                     "update",
			Description:              "参加,退出を通知します",
			DefaultMemberPermissions: omit.NewPtr(discord.PermissionViewChannel),
		},
	}
	_, err := r.Client().Rest.SetGlobalCommands(clientID, commands)
	if err != nil {
		panic(err)
	}

	// Get Owners
	app, err := r.Client().Rest.GetBotApplicationInfo()
	if err == nil {
		if app.Team != nil {
			for _, member := range app.Team.Members {
				owners = append(owners, member.User.ID)
			}
		} else {
			owners = append(owners, app.Owner.ID)
		}
	}

}

func onMessageCreate(m *events.MessageCreate) {
	client := m.Client()
	// bot status update
	joinedGuilds := client.Caches.GuildsLen()
	joinedVC := len(sessions.guilds)
	client.SetPresence(
		context.Background(),
		gateway.WithOnlineStatus(discord.OnlineStatusOnline),
		gateway.WithListeningActivity(
			"i'm a bot",
			gateway.WithActivityState(fmt.Sprintf("Working on %d servers (Speech for %d servers)", joinedGuilds, joinedVC)),
		),
	)

	logger.Info(toJson(m))
	if m.Message.Author.Bot {
		return
	}

	// Check reading skip
	if m.Message.Content == "" || strings.HasPrefix(m.Message.Content, ";") {
		return
	}

	// debug
	if slices.Contains(owners, m.Message.Author.ID) {
		switch {
		case utils.RegMatch(m.Message.Content, "^!debug"):
			// Session delete
			if utils.RegMatch(m.Message.Content, "[0-9]$") {
				guildID := utils.RegReplace(m.Message.Content, "", `^!debug\s*`)
				logger.Info("Deleting SessionItem : " + guildID)
				id, _ := snowflake.Parse(guildID)
				sessions.Delete(&id)
				return
			}

			// Voice channel user list
			VCdata := map[snowflake.ID][]string{}

			for guild := range client.Caches.Guilds() {
				for vs := range client.Caches.VoiceStates(guild.ID) {
					user, ok := client.Caches.Member(vs.GuildID, vs.UserID)
					if !ok {
						continue
					}

					VCdata[vs.GuildID] = append(VCdata[vs.GuildID], user.EffectiveName())
				}
			}

			// Return voice connection information
			for _, session := range sessions.guilds {
				guild, ok := client.Caches.Guild(*session.guildID)
				if !ok {
					utils.PrintError("Failed Get GuildData by GuildID", fmt.Errorf("cache failed"))
					continue
				}

				channel, ok := client.Caches.Channel(session.channelID)
				if !ok {
					utils.PrintError("Failed Get ChannelData by ChannelID", fmt.Errorf("cache failed"))
					continue
				}

				embed, err := client.Rest.CreateMessage(
					m.ChannelID,
					discord.NewMessageCreate().
						AddEmbeds(discord.Embed{
							Type:        discord.EmbedTypeRich,
							Title:       fmt.Sprintf("Guild:%s(%s)\nChannel:%s(%s)", guild.Name, session.guildID, channel.Name(), session.channelID),
							Description: fmt.Sprintf("Members:```\n%s```", strings.Join(VCdata[guild.ID], ",")),
							Color:       embedColor,
						}),
				)

				if err == nil {
					go func() {
						time.Sleep(30 * time.Second)
						err := client.Rest.DeleteMessage(m.ChannelID, embed.ID)
						utils.PrintError("failed delete debug message", err)
					}()
				}
			}
			if len(sessions.guilds) == 0 {
				embed, err := client.Rest.CreateMessage(
					m.ChannelID,
					discord.NewMessageCreate().
						AddEmbeds(discord.Embed{
							Type:  discord.EmbedTypeRich,
							Title: "Session Not Found",
							Color: embedColor,
						}),
				)
				if err == nil {
					go func() {
						time.Sleep(30 * time.Second)
						err := client.Rest.DeleteMessage(m.ChannelID, embed.ID)
						utils.PrintError("failed delete debug message", err)
					}()
				}
			}
			return
		}
	}

	//読み上げ
	session := sessions.Get(m.GuildID)
	if session != nil {
		if session.IsJoined() && session.channelID == m.ChannelID {
			session.Speech(m.Message.Author.ID, m.Message.Content)
			return
		}
	}
}

// InteractionCreate
func onInteractionCreate(i *events.ApplicationCommandInteractionCreate) {
	// 表示&処理しやすく
	logger.Info(toJson(i))

	// 分岐
	data := i.SlashCommandInteractionData()
	switch data.CommandName() {
	//TTS
	case "join":
		i.DeferCreateMessage(false)

		session := sessions.Get(i.GuildID())
		if session.IsJoined() {
			sessions.Failed(i, "VoiceChat にすでに接続しています")
			return
		}

		session.JoinVoice(i)
		return

	case "leave":
		i.DeferCreateMessage(false)

		session := sessions.Get(i.GuildID())
		if !session.IsJoined() {
			sessions.Failed(i, "VoiceChat に接続していません")
			return
		}
		session.LeaveVoice(i)

	case "get":
		i.DeferCreateMessage(false)

		result, err := sessions.Config(i.User().ID, UserSetting{})
		if utils.PrintError("Failed Get Config", err) {
			sessions.Failed(i, "データのアクセスに失敗しました。")
			return
		}

		i.Client().Rest.CreateFollowupMessage(
			i.ApplicationID(),
			i.Token(),
			discord.NewMessageCreate().AddEmbeds(
				discord.Embed{
					Title:       fmt.Sprintf("@%s's Speech Config", i.User().Username),
					Color:       embedColor,
					Description: fmt.Sprintf("```\nLang  : %4s\nSpeed : %3.2f\nPitch : %3.2f```", result.Lang, result.Speed, result.Pitch),
				},
			),
		)
		return

		// case "set":
		// 	i.DeferCreateMessage(false)

		// 	sessions.UpdateConfig(res, iData)
		// 	return

		// case "dic":
		// 	i.DeferCreateMessage(false)

		// 	session := sessions.Get(i.GuildID())
		// 	if !session.IsJoined() {
		// 		sessions.Failed(i, "VoiceChat に接続していません")
		// 		return
		// 	}

		// 	session.Dictionary(i)
		// 	return

		// case "update":
		// 	i.DeferCreateMessage(false)

		// 	session := sessions.Get(i.GuildID())
		// 	if !session.IsJoined() {
		// 		sessions.Failed(i, "VoiceChat に接続していません")
		// 		return
		// 	}

		// 	session.ToggleUpdate(res)
		// 	return
	}
}

// // VCでJoin||Leaveが起きたときにCall
// func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
// 	vData := disgord.VoiceStateParse(discord, v)
// 	if !vData.UpdateStatus.ChannelJoin {
// 		return
// 	}
// 	logger.Info(toJson(v))

// 	//セッションがあるか確認
// 	session := sessions.Get(v.GuildID)
// 	if session == nil {
// 		return
// 	}
// 	session.AutoLeave(discord, vData.Status.ChannelJoin, vData.User.Username)
// }

func toJson(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Sessions) Get(guildID *snowflake.ID) *SessionData {
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

func (s *Sessions) Delete(guildID *snowflake.ID) {
	s.save.Lock()
	defer s.save.Unlock()
	var newSessions []*SessionData
	for _, session := range s.guilds {
		if session.guildID == guildID {
			if session.vc != nil {
				(*session.vc).Close(context.Background())
			}
			continue
		}
		newSessions = append(newSessions, session)
	}
	s.guilds = newSessions
}

func (s *SessionData) JoinVoice(i *events.ApplicationCommandInteractionCreate) {
	guildID := i.GuildID()
	if guildID == nil {
		sessions.Failed(i, "このコマンドはサーバー内でのみ実行できます。")
		return
	}

	voiceState, found := i.Client().Caches.VoiceState(*guildID, i.User().ID)
	if !found || voiceState.ChannelID == nil {
		// ユーザーがボイスチャンネルに入っていない場合
		sessions.Failed(i, "ユーザーの接続している VoiceChat を見つけられませんでした。")
		return
	}

	vcSession := i.Client().VoiceManager.CreateConn(*guildID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)

	defer cancel()
	err := vcSession.Open(ctx, *voiceState.ChannelID, false, false)
	if err != nil {
		logger.Error("Failed to open voice conn: " + err.Error())
		sessions.Failed(i, "Voice Connection を確立できませんでした。")
		return
	}

	vcSession.Gateway().Status()
	// vcSession.LogLevel = discordgo.LogDebug

	session := &SessionData{
		guildID:   guildID,
		channelID: *voiceState.ChannelID,
		vc:        &vcSession,
		lead:      sync.Mutex{},
	}

	sessions.Add(session)

	session.Speech(0, "おはー")
	sessions.Success(i, "ハロー!")
}

func (s *SessionData) LeaveVoice(i *events.ApplicationCommandInteractionCreate) {
	s.Speech(0, "さいなら")
	sessions.Success(i, "グッバイ!")
	time.Sleep(1 * time.Second)
	(*s.vc).Close(context.Background())

	sessions.Delete(s.guildID)
}

// func (s *SessionData) AutoLeave(discord *discordgo.Session, isJoin bool, userName string) {
// 	checkVcChannelID := (*s.vc).ChannelID()

// 	// ボイスチャンネルに誰かいるか
// 	isLeave := true
// 	for _, guild := range discord.State.Guilds {
// 		for _, vs := range guild.VoiceStates {
// 			id, _ := snowflake.Parse(vs.UserID)
// 			if checkVcChannelID == vs..ChannelID && id != clientID {
// 				isLeave = false
// 				break
// 			}
// 		}
// 	}

// 	if isLeave {
// 		// ボイスチャンネルに誰もいなかったら Disconnect する
// 		(*s.vc).Close(context.Background())
// 		sessions.Delete(s.guildID)
// 	} else {
// 		// でなければ通知?
// 		if !s.updateInfo {
// 			return
// 		}
// 		if isJoin {
// 			s.Speech(0, fmt.Sprintf("%s join the voice", userName))
// 		} else {
// 			s.Speech(0, fmt.Sprintf("%s left the voice", userName))
// 		}
// 	}
// }

func (session *SessionData) Speech(userID snowflake.ID, text string) {
	if session.CheckDic() {
		data, _ := os.Open(filepath.Join(".", "dic", session.guildID.String()+".txt"))
		defer data.Close()

		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			if scanner.Err() != nil {
				break
			}
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

	// 読み上げ待機
	session.lead.Lock()
	defer session.lead.Unlock()

	// 以前の disgord.PlayAudioFile 相当の呼び出し
	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%s&tl=%s", url.QueryEscape(read), settingData.Lang)

	// disgo の voice.Conn (session.vc と想定) に対して再生
	err = PlayAudioURL(context.Background(), *session.vc, voiceURL, settingData.Speed, settingData.Pitch, 1.0)
	utils.PrintError("Failed play Audio \""+read+"\" ", err)
}

// func (s *SessionData) Dictionary(res *disgord.InteractionResponse, i disgord.InteractionData) {
// 	//ファイルの指定
// 	fileName := filepath.Join(".", "dic", s.guildID.String()+".txt")
// 	//dicがあるか確認
// 	if !s.CheckDic() {
// 		sessions.Failed(res, "辞書の読み込みに失敗しました")
// 		return
// 	}

// 	textByte, _ := os.ReadFile(fileName)
// 	dic := string(textByte)

// 	//textをfrom toに
// 	from := i.CommandOptions["from"].StringValue()
// 	to := i.CommandOptions["to"].StringValue()

// 	// 禁止文字チェック
// 	if strings.Contains(from, ",") || strings.Contains(to, ",") {
// 		sessions.Failed(res, "使用できない文字が含まれています")
// 		return
// 	}

// 	//確認
// 	if strings.Contains(dic, from+",") {
// 		dic = utils.RegReplace(dic, "", "\n"+from+",.*")
// 	}
// 	dic = dic + from + "," + to + "\n"

// 	//書き込み
// 	err := os.WriteFile(fileName, []byte(dic), 0755)
// 	if utils.PrintError("Config Update Failed", err) {
// 		sessions.Failed(res, "辞書の書き込みに失敗しました")
// 		return
// 	}

// 	sessions.Success(res, "辞書を保存しました\n\""+from+"\" => \""+to+"\"")
// }

// func (s *SessionData) ToggleUpdate(res *disgord.InteractionResponse) {
// 	s.updateInfo = !s.updateInfo

// 	sessions.Success(res, fmt.Sprintf("ボイスチャットの参加/退出の通知を %t に変更しました", s.updateInfo))
// }

func (s *SessionData) CheckDic() (ok bool) {
	// dic.txtがあるか
	_, err := os.Stat(filepath.Join(".", "dic", s.guildID.String()+".txt"))
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
	f, err := os.Create(filepath.Join(".", "dic", s.guildID.String()+".txt"))
	f.Close()
	return !utils.PrintError("Failed create dictionary", err)
}

func (s *Sessions) Config(userID snowflake.ID, newConfig UserSetting) (result UserSetting, err error) {
	//BOTチェック
	if userID == 0 {
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

	Users := map[snowflake.ID]UserSetting{}
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

// func (s *Sessions) UpdateConfig(res *disgord.InteractionResponse, i disgord.InteractionData) (ok string, err error) {
// 	// 読み込み
// 	result, err := sessions.Config(i.User.ID, UserSetting{})
// 	if utils.PrintError("Failed Get Config", err) {
// 		sessions.Failed(res, "読み上げ設定を読み込めませんでした")
// 		return
// 	}
// 	// チェック
// 	if newSpeed, ok := i.CommandOptions["speed"]; ok {
// 		result.Speed = newSpeed.FloatValue()
// 	}
// 	if newPitch, ok := i.CommandOptions["pitch"]; ok {
// 		result.Pitch = newPitch.FloatValue()
// 	}
// 	if newLang, ok := i.CommandOptions["lang"]; ok {
// 		result.Lang = newLang.StringValue()
// 		// 言語チェック
// 		_, err := language.Parse(result.Lang)
// 		if result.Lang != "auto" && err != nil {
// 			s.Failed(res, "不明な言語です\n\"auto\"もしくは言語コードのみ使用可能です")
// 			return
// 		}
// 	}

// 	_, err = sessions.Config(i.User.ID, result)
// 	if utils.PrintError("Failed Write Config", err) {
// 		sessions.Failed(res, "保存に失敗しました")
// 	}
// 	sessions.Success(res, "読み上げ設定を変更しました")
// }

func (s *Sessions) Failed(i *events.ApplicationCommandInteractionCreate, description string) {
	_, err := i.Client().Rest.CreateFollowupMessage(
		i.ApplicationID(),
		i.Token(),
		discord.NewMessageCreate().AddEmbeds(
			discord.Embed{
				Title:       "Command Failed",
				Color:       embedColor,
				Description: description,
			},
		),
	)
	utils.PrintError("Failed send response", err)
}

func (s *Sessions) Success(i *events.ApplicationCommandInteractionCreate, description string) {
	_, err := i.Client().Rest.CreateFollowupMessage(
		i.ApplicationID(),
		i.Token(),
		discord.NewMessageCreate().AddEmbeds(
			discord.Embed{
				Title:       "Command Success",
				Color:       embedColor,
				Description: description,
			},
		),
	)
	utils.PrintError("Failed send response", err)
}

func PlayAudioURL(ctx context.Context, conn voice.Conn, filename string, speed float64, pitch float64, volume float64) error {
	// 1. ボイスゲートウェイの接続が「Ready (接続完了)」になるまで最大5秒間待機する
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()

	for {
		// Gateway が存在し、ステータスが StatusReady (接続完了) になっているかチェック
		if conn.Gateway() != nil && conn.Gateway().Status() == voice.StatusReady {
			break
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("voice connection timeout: gateway is not readyaaaaaaa")
		default:
			time.Sleep(50 * time.Millisecond) // 50msごとにチェック
		}
	}

	// 1. 発言中状態にする
	if err := conn.SetSpeaking(ctx, voice.SpeakingFlagMicrophone); err != nil {
		return fmt.Errorf("failed to set speaking flag: %w", err)
	}
	defer func() {
		// 再生終了時に発言フラグを下げる
		_ = conn.SetSpeaking(ctx, voice.SpeakingFlagNone)
	}()

	// 2. FFmpeg を起動して DCA（Discord 互換 Opus）フォーマットに変換する
	// 以前の aresample フィルタをベースに調整します
	filter := fmt.Sprintf("aresample=48000,asetrate=48000*%.2f/100,atempo=100/%.2f*%.2f,volume=%.2f", pitch*100, pitch*100, speed, volume)

	cmd := exec.Command("ffmpeg",
		"-i", filename, // 入力（HTTP の URL も直接指定可能）
		"-af", filter, // 速度・ピッチ・音量のフィルタ
		"-f", "opus", // 出力は opus
		"-ar", "48000", // 48000Hz (Discord仕様)
		"-ac", "2", // ステレオ 2ch (Discord仕様)
		"-b:a", "96k", // ビットレート
		"-application", "audio",
		"-frame_duration", "20", // 20ms フレーム
		"-f", "data", // 各パケットの前にサイズ(4バイト little-endian)を付加する
		"pipe:1", // 標準出力へパイプ
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	defer func() {
		// プロセスを終了させる
		_ = stdout.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// 3. disgo の NewOpusReader を使って、FFmpegの標準出力を Opus フレームプロバイダーに変換
	// (NewOpusReader は 4バイトの little-endian サイズヘッダを期待して動作します)
	provider := voice.NewOpusReader(stdout)

	// 4. コネクションにプロバイダーを設定して再生を開始
	conn.SetOpusFrameProvider(provider)

	// 5. 再生が終わる、または context がキャンセルされるまで待機する
	// ※SetOpusFrameProvider はバックグラウンドスレッドで自動で 20ms タイマーを回してくれるため、
	// 呼び出し元のこのスレッドは、FFmpeg 側が EOF になるまでブロックして待つだけでOKです。
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 外部から中断された場合
			conn.SetOpusFrameProvider(nil)
			return ctx.Err()
		case <-ticker.C:
		}

		// FFmpegが終了して io.EOF を検知したかどうかを、プロバイダー経由で監視（またはcmd.Wait）
		// ここでは、FFmpeg プロセスの終了ステータスで監視します
		state, err := cmd.Process.Wait()
		if err == nil || state != nil {
			break // 再生終了
		}
	}

	// プロバイダーを外してクリーンアップ
	conn.SetOpusFrameProvider(nil)
	return nil
}
