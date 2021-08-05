package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/takanakahiko/discord-tts/session"
)

var (
	sessionManager = session.NewTtsSessionManager()
	prefix         = flag.String("prefix", "", "call prefix")
	token          = flag.String("token", "", "bot token")
	clientID       = ""
)

func main() {
	flag.Parse()
	fmt.Println("prefix       :", *prefix)
	fmt.Println("token        :", *token)

	discord, err := discordgo.New()
	if err != nil {
		fmt.Println("Error logging in")
		fmt.Println(err)
	}

	discord.Token = "Bot " + *token
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onVoiceStateUpdate)

	if err = discord.Open(); err != nil {
		fmt.Println(err)
	}
	defer func() {
		if err := discord.Close(); err != nil {
			log.Println(err)
		}
	}()

	fmt.Println("Listening...")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID
	discord.UpdateStatus(0,*prefix+" help")
}

//event by message
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {


	discordChannel, err := discord.Channel(m.ChannelID)
	if err != nil {
		log.Fatal(err)
		return
	}

	guild, err := discord.Guild(m.GuildID)
	if err != nil && err != session.ErrTtsSessionNotFound {
		log.Println(err)
		return
	}
	//logのやつ
	log.Println("server:\""+guild.Name+"\"    ch:"+discordChannel.Name+"    user:"+m.Author.Username+"    message:"+m.Content)

	//bot 読み上げ無し のチェック
	if m.Author.Bot || strings.HasPrefix(m.Content, ";") {
		return
	}

	if PrefixCheck(m.Content, "help") {
		discord.ChannelMessageSend(m.ChannelID, "\n使用可能コマンド:\n"+*prefix+" join : VCに接続します\n"+*prefix+" speed <speech speed> : 読み上げ速度を変更します\n"+*prefix+" lang <language code> : 読み上げ言語を変更します\n"+*prefix+" limit <speech limit> : 読み上げ文字数の上限を設定します\n"+*prefix+" leave : VCから切断します")
		return
	}

	// "join" command
	if PrefixCheck(m.Content, "join") {
		_, err := sessionManager.GetByGuidID(m.GuildID)
		if err != nil && err != session.ErrTtsSessionNotFound {
			log.Println(err)
			return
		}
		if err == nil {
			sendMessage(discord, m.ChannelID, "Bot is already in voice-chat.")
			return
		}
		ttsSession := session.NewTtsSession()
		if err := ttsSession.Join(discord, m.Author.ID, m.ChannelID); err != nil {
			if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"❌"); err!= nil {
				log.Println(err)
			}	
			log.Println(err)
			return
		}
		if err = sessionManager.Add(ttsSession); err != nil {
			log.Println(err)
		}
		if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"✅"); err!= nil {
			log.Println(err)
		}
		return
	}

	// ignore case of "not join" or "include ignore prefix"
	ttsSession, err := sessionManager.GetByGuidID(m.GuildID)
	if err == session.ErrTtsSessionNotFound {
		return
	}
	if err != nil {
		log.Println(err)
		return
	}

	// Ignore if the TextChanelID of session and the channel of the message are different
	if ttsSession.TextChanelID != m.ChannelID {
		return
	}

	// other commands
	switch {
	case PrefixCheck(m.Content, "leave"):
		if err := ttsSession.Leave(discord); err != nil {
			log.Println(err)
		}
		if err := sessionManager.Remove(ttsSession.GuidID()); err != nil {
			log.Println(err)
		}
		if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"⛔"); err!= nil {
			log.Println(err)
		}
		return
	case PrefixCheck(m.Content, "speed"):
		speedStr := strings.Replace(m.Content, *prefix+" speed ", "", 1)
		newSpeed, err := strconv.ParseFloat(speedStr, 64)
		if err != nil {
			ttsSession.SendMessage(discord, "数字ではない値は設定できません")
			return
		}
		if err = ttsSession.SetSpeechSpeed(discord, newSpeed); err != nil {
			log.Println(err)
			return
		}
		if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"🔊"); err!= nil {
			log.Println(err)
		}
		return
	case PrefixCheck(m.Content, "lang"):
		newLang := strings.Replace(m.Content, *prefix+" lang ", "", 1)
		if err = ttsSession.SetLanguage(discord, newLang); err != nil {
			if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"❌"); err!= nil {
				log.Println(err)
			}	
			log.Println(err)
			return
		}
		if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"🗣️"); err!= nil {
			log.Println(err)
		}
		return
	case PrefixCheck(m.Content, "limit"):
		LimitStr := strings.Replace(m.Content, *prefix+" limit ", "", 1)
		newLimit, err := strconv.Atoi(LimitStr)
		if err != nil {
			ttsSession.SendMessage(discord, "数字ではない値は設定できません")
			return
		}
		if err = ttsSession.SetSpeechLimit(discord, newLimit); err != nil {
			if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"❌"); err!= nil {
				log.Println(err)
			}	
			log.Println(err)
			return
		}
		if err := discord.MessageReactionAdd(m.ChannelID,m.ID,"🥺"); err!= nil {
			log.Println(err)
		}
		return
	}

	if err = ttsSession.Speech(discord, m.Content); err != nil {
		log.Println(err)
	}
}

func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	ttsSession, err := sessionManager.GetByGuidID(v.GuildID)
	if err == session.ErrTtsSessionNotFound {
		return
	}
	if err != nil {
		log.Println(err)
		return
	}

	if ttsSession.VoiceConnection == nil || !ttsSession.VoiceConnection.Ready {
		return
	}

	// ボイスチャンネルに誰かしらいたら return
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if ttsSession.VoiceConnection.ChannelID == vs.ChannelID && vs.UserID != clientID {
				return
			}
		}
	}

	// ボイスチャンネルに誰もいなかったら Disconnect する
	if err := sessionManager.Remove(v.GuildID); err != nil {
		log.Println(err)
	}
	err = ttsSession.Leave(discord)
	if err != nil {
		log.Println(err)
	}
}

func PrefixCheck(message, command string) bool {
	return strings.HasPrefix(message, *prefix+" "+command)
}

func sendMessage(discord *discordgo.Session, textChanelID, format string, v ...interface{}) {
	session := session.NewTtsSession()
	session.TextChanelID = textChanelID
	session.SendMessage(discord, format, v...)
}