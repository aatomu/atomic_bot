package main

import (
	"flag"
	"fmt"
	"time"

	"log"
	"strings"

	"github.com/aatomu/atomicgo/discordbot"
	"github.com/aatomu/atomicgo/utils"
	"github.com/aatomu/slashlib"
	"github.com/bwmarrin/discordgo"
)

var (
	//変数定義
	clientID              = ""
	token                 = flag.String("token", "", "bot token")
	ttsSession            ttsSessions
	isVcSessionUpdateLock = false
	dummy                 = UserSetting{
		Lang:  "auto",
		Speed: 1.5,
		Pitch: 1.1,
	}
	embedColor = 0x1E90FF
)

func main() {
	//flag入手
	flag.Parse()
	fmt.Println("token        :", *token)

	//bot起動準備
	discord, err := discordbot.Init(*token)
	if err != nil {
		fmt.Println("Failed Bot Init", err)
		return
	}

	//eventトリガー設定
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onInteractionCreate)
	discord.AddHandler(onVoiceStateUpdate)

	//起動
	discordbot.Start(discord)
	defer func() {
		for _, session := range ttsSession.guilds {
			discord.ChannelMessageSendEmbed(session.channelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "__Infomation__",
				Description: "Sorry. Bot will Shutdown. Will be try later.",
				Color:       embedColor,
			})
		}
		discord.Close()
	}()
	//起動メッセージ表示
	fmt.Println("Listening...")

	//bot停止対策
	<-utils.BreakSignal()
}

// BOTの準備が終わったときにCall
func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID

	// コマンドの追加
	var minSpeed float64 = 0.5
	var minPitch float64 = 0.5
	new(slashlib.Command).
		//TTS
		AddCommand("join", "VoiceChatに接続します", discordgo.PermissionViewChannel).
		AddCommand("leave", "VoiceChatから切断します", discordgo.PermissionViewChannel).
		AddCommand("get", "読み上げ設定を表示します", discordgo.PermissionViewChannel).
		AddCommand("set", "読み上げ設定を変更します", discordgo.PermissionViewChannel).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionNumber,
			Name:        "speed",
			Description: "読み上げ速度を設定",
			MinValue:    &minSpeed,
			MaxValue:    5,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionNumber,
			Name:        "pitch",
			Description: "声の高さを設定",
			MinValue:    &minPitch,
			MaxValue:    1.5,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "lang",
			Description: "読み上げ言語を設定",
		}).
		AddCommand("dic", "辞書を設定します", discordgo.PermissionViewChannel).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "from",
			Description: "置換元",
			Required:    true,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "to",
			Description: "置換先",
			Required:    true,
		}).
		AddCommand("update", "参加,退出を通知します", discordgo.PermissionViewChannel).
		//その他
		AddCommand("poll", "投票を作成します", discordgo.PermissionViewChannel).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "title",
			Description: "投票のタイトル",
			Required:    true,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_1",
			Description: "選択肢 1",
			Required:    true,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_2",
			Description: "選択肢 2",
			Required:    true,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_3",
			Description: "選択肢 3",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_4",
			Description: "選択肢 4",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_5",
			Description: "選択肢 5",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_6",
			Description: "選択肢 6",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_7",
			Description: "選択肢 7",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_8",
			Description: "選択肢 8",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_9",
			Description: "選択肢 9",
			Required:    false,
		}).
		AddOption(&discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choice_10",
			Description: "選択肢 10",
			Required:    false,
		}).
		CommandCreate(discord, "")
}

// メッセージが送られたときにCall
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	// state update
	joinedGuilds := len(discord.State.Guilds)
	joinedVC := len(ttsSession.guilds)
	VC := ""
	if joinedVC != 0 {
		VC = fmt.Sprintf(" %d鯖でお話し中", joinedVC)
	}
	discordbot.BotStateUpdate(discord, fmt.Sprintf("/join | %d鯖で稼働中 %s", joinedGuilds, VC), 0)

	mData := discordbot.MessageParse(discord, m)
	log.Println(mData.FormatText)

	// VCsession更新
	go func() {
		if isVcSessionUpdateLock {
			return
		}

		// 更新チェック
		isVcSessionUpdateLock = true
		defer func() {
			time.Sleep(1 * time.Minute)
			isVcSessionUpdateLock = false
		}()

		for i := range ttsSession.guilds {
			go func(n int) {
				session := ttsSession.guilds[n]
				session.lead.Lock()
				defer session.lead.Unlock()
				session.vc = discord.VoiceConnections[session.guildID]
			}(i)
		}
	}()

	// 読み上げ無し のチェック
	if strings.HasPrefix(m.Content, ";") {
		return
	}

	// debug
	if mData.UserID == "701336137012215818" {
		switch {
		case utils.RegMatch(mData.Message, "^!debug"):
			// セッション処理
			if utils.RegMatch(mData.Message, "[0-9]$") {
				guildID := utils.RegReplace(mData.Message, "", `^!debug\s*`)
				log.Println("Deleting SessionItem : " + guildID)
				ttsSession.Delete(guildID)
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
			for _, session := range ttsSession.guilds {
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
			return
		}
	}

	//読み上げ
	session := ttsSession.Get(mData.GuildID)
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

	// response用データ
	res := slashlib.InteractionResponse{
		Discord:     discord,
		Interaction: iData.Interaction,
	}

	// 分岐
	switch i.Command.Name {
	//TTS
	case "join":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat にすでに接続しています")
			return
		}

		session.JoinVoice(res, i.GuildID, i.ChannelID, i.UserID)
		return

	case "leave":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if !session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat に接続していません")
			return
		}

	case "get":
		res.Thinking(false)

		result, err := ttsSession.Config(i.UserID, UserSetting{})
		if utils.PrintError("Failed Get Config", err) {
			ttsSession.Failed(res, "データのアクセスに失敗しました。")
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

		ttsSession.UpdateConfig(res, i)
		return

	case "dic":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if !session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat に接続していません")
			return
		}

		session.Dictionary(res, i)
		return

	case "update":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if !session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat に接続していません")
			return
		}

		session.ToggleUpdate(res)
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
					Color:       embedColor,
					Description: description,
				},
			},
		})
		if utils.PrintError("Failed Follow", err) {
			return
		}
		time.Sleep(1 * time.Second)
		for i := 0; i < len(choices); i++ {
			discord.MessageReactionAdd(m.ChannelID, m.ID, reaction[i])
		}
	}
}

// VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	vData := discordbot.VoiceStateParse(discord, v)
	if !vData.StatusUpdate.ChannelJoin {
		return
	}
	log.Println(vData.FormatText)

	//セッションがあるか確認
	session := ttsSession.Get(v.GuildID)
	if session == nil {
		return
	}
	session.AutoLeave(discord, vData.Status.ChannelJoin, vData.UserName)
}
