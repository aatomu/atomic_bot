package main

import (
	"flag"
	"fmt"
	"time"

	"log"
	"strings"

	"github.com/aatomu/aatomlib/disgord"
	"github.com/aatomu/aatomlib/utils"
	"github.com/bwmarrin/discordgo"
)

var (
	//変数定義
	clientID              = ""
	token                 = flag.String("token", "", "bot token")
	ttsSession            ttsSessions
	blackjack             BlackjackSessions
	isVcSessionUpdateLock = false
	dummy                 = UserSetting{
		Lang:  "auto",
		Speed: 1.5,
		Pitch: 1.1,
	}
	embedColor = 0x1E90FF
)

func main() {
	flag.Parse()
	fmt.Println("token        :", *token)

	// Initialize bot
	discord, err := discordgo.New("Bot " + *token)
	if err != nil {
		fmt.Println("Failed Bot Init", err)
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
		for _, session := range ttsSession.guilds {
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
		// Others
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "poll",
			Description:              "投票を作成します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "投票のタイトル", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_1", Description: "選択肢 1", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_1", Description: "選択肢 1", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_2", Description: "選択肢 2", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_3", Description: "選択肢 3", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_4", Description: "選択肢 4", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_5", Description: "選択肢 5", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_6", Description: "選択肢 6", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_7", Description: "選択肢 7", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_8", Description: "選択肢 8", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_9", Description: "選択肢 9", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "choice_10", Description: "選択肢 10", Required: false},
			},
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "simple-poll",
			Description:              "簡易的な投票を作成",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "text", Description: "メッセージ内容", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reaction_1", Description: "リアクション 1", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reaction_2", Description: "リアクション 2", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reaction_3", Description: "リアクション 3", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reaction_4", Description: "リアクション 4", Required: false},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reaction_5", Description: "リアクション 5", Required: false},
			},
		},
		{
			Type:                     discordgo.ChatApplicationCommand,
			Name:                     "blackjack",
			Description:              "blackjackを開始します",
			DefaultMemberPermissions: Pinter(discordgo.PermissionViewChannel),
		},
	})
}

func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	// bot status update
	joinedGuilds := len(discord.State.Guilds)
	joinedVC := len(ttsSession.guilds)
	discord.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name:    "i'm a bot",
				Type:    discordgo.ActivityTypeCustom,
				Details: "Working for everyone",
				State:   "`/join` `/poll` Talking for",
				Party: discordgo.Party{
					ID:   "-1",
					Size: []int{joinedVC, joinedGuilds},
				},
			},
		},
	})

	mData := disgord.MessageParse(discord, m.Message)
	log.Println(mData.FormatText)

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

		for i := range ttsSession.guilds {
			go func(n int) {
				session := ttsSession.guilds[n]
				session.lead.Lock()
				defer session.lead.Unlock()
				session.vc = discord.VoiceConnections[session.guildID]
			}(i)
		}
	}()

	// Check reading skip
	if strings.HasPrefix(m.Content, ";") {
		return
	}

	// debug
	if mData.User.ID == "701336137012215818" {
		switch {
		case utils.RegMatch(mData.Message.Content, "^!debug"):
			// Session delete
			if utils.RegMatch(mData.Message.Content, "[0-9]$") {
				guildID := utils.RegReplace(mData.Message.Content, "", `^!debug\s*`)
				log.Println("Deleting SessionItem : " + guildID)
				ttsSession.Delete(guildID)
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
	if session != nil {
		if session.IsJoined() && session.channelID == mData.ChannelID && !(m.Author.Bot && !session.enableBot) {
			session.Speech(mData.User.ID, mData.Message.Content)
			return
		}
	}
}

// InteractionCreate
func onInteractionCreate(discord *discordgo.Session, i *discordgo.InteractionCreate) {
	// 表示&処理しやすく
	iData := disgord.InteractionParse(discord, i.Interaction)

	// response用データ
	res := disgord.NewInteractionResponse(discord, i.Interaction)

	// 分岐
	switch iData.Command.Name {
	//TTS
	case "join":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat にすでに接続しています")
			return
		}

		session.JoinVoice(res, discord, i.GuildID, i.ChannelID, i.User.ID)
		return

	case "leave":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if !session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat に接続していません")
			return
		}
		session.LeaveVoice(res)

	case "get":
		res.Thinking(false)

		result, err := ttsSession.Config(iData.User.ID, UserSetting{})
		if utils.PrintError("Failed Get Config", err) {
			ttsSession.Failed(res, "データのアクセスに失敗しました。")
			return
		}

		res.Follow(&discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       fmt.Sprintf("@%s's Speech Config", i.User.Username),
					Description: fmt.Sprintf("```\nLang  : %4s\nSpeed : %3.2f\nPitch : %3.2f```", result.Lang, result.Speed, result.Pitch),
				},
			},
		})
		return

	case "set":
		res.Thinking(false)

		ttsSession.UpdateConfig(res, iData)
		return

	case "dic":
		res.Thinking(false)

		session := ttsSession.Get(i.GuildID)
		if !session.IsJoined() {
			ttsSession.Failed(res, "VoiceChat に接続していません")
			return
		}

		session.Dictionary(res, iData)
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

		title := iData.CommandOptions["title"].StringValue()
		choices := []string{}
		choices = append(choices, iData.CommandOptions["choice_1"].StringValue())
		choices = append(choices, iData.CommandOptions["choice_2"].StringValue())
		if value, ok := iData.CommandOptions["choice_3"]; ok {
			choices = append(choices, value.StringValue())
		}
		if value, ok := iData.CommandOptions["choice_4"]; ok {
			choices = append(choices, value.StringValue())
		}
		if value, ok := iData.CommandOptions["choice_5"]; ok {
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
		//その他
	case "simple-poll":
		res.Reply(nil)

		text := iData.CommandOptions["text"].StringValue()
		reactions := []string{}
		for x := 1; x <= 5; x++ {
			v, ok := iData.CommandOptions[fmt.Sprintf("reaction_%d", x)]
			if !ok {
				continue
			}
			reactions = append(reactions, v.StringValue())
		}

		m, err := discord.ChannelMessageSend(i.ChannelID, text)
		if err != nil {
			return
		}
		time.Sleep(1 * time.Second)
		for _, reaction := range reactions {
			discord.MessageReactionAdd(m.ChannelID, m.ID, reaction)
		}
	case "blackjack":
		// Session Check
		session := blackjack.Get(i.GuildID)
		if session != nil {
			session.Failed(res, "すでに blackjackが存在します")
		}

		session.NewGame(res, discord, i.GuildID, i.ChannelID)
		return
	}

	switch iData.Component.CustomID {
	case "blackjack-game-join":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}
		if session.phase != Wait {
			res.Reply(nil)
			return
		}

		session.GameJoin(res, discord, i.User.Username)

	case "blackjack-game-leave":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != Wait {
			res.Reply(nil)
			return
		}

		session.GameLeave(res, discord, i.User.Username)
		return

	case "blackjack-game-start":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != Wait {
			res.Reply(nil)
			return
		}

		session.GameStart(res, discord, i.User.Username)
		return

	case "blackjack-bed-call":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != BetTime {
			res.Reply(nil)
			return
		}

		session.BetCall(res, i.User.Username)
		return

	case "blackjack-bed-close":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != BetTime {
			res.Reply(nil)
			return
		}

		session.BetClose(res, discord, i.User.Username)
		return

	case "blackjack-card-hit":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != HitTime {
			res.Reply(nil)
			return
		}

		session.CardHit(res, discord, i.User.Username)
		return

	case "blackjack-card-finish":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != HitTime {
			res.Reply(nil)
			return
		}

		session.CardFinish(res, discord, i.User.Username)
		return

	case "blackjack-game-continue":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != Ended {
			res.Reply(nil)
			return
		}

		session.GameContinue(res, discord, i.User.Username)
		return

	case "blackjack-game-finish":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != Ended {
			res.Reply(nil)
			return
		}

		session.GameFinish(res, discord, i.User.Username)
		blackjack.Delete(i.GuildID)
		return
	}

	switch iData.Modal.CustomID {
	case "blackjack-bed-input":
		session := blackjack.Get(i.GuildID)
		if session == nil {
			session.Failed(res, "現在 blackjack が行われていません")
			return
		}

		if session.phase != BetTime {
			res.Reply(nil)
			return
		}
		session.UpdateBetValue(res, discord, i.User.Username, i.ModalSubmitData())
		return
	}
}

// VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	vData := disgord.VoiceStateParse(discord, v)
	if !vData.UpdateStatus.ChannelJoin {
		return
	}
	log.Println(vData.FormatText)

	//セッションがあるか確認
	session := ttsSession.Get(v.GuildID)
	if session == nil {
		return
	}
	session.AutoLeave(discord, vData.Status.ChannelJoin, vData.User.Username)
}

func Pinter(n int64) *int64 {
	return &n
}
