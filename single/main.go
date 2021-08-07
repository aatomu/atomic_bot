package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"sync"
	"time"

	"golang.org/x/text/language"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)





var (
	//関数定義
	prefix                                  = flag.String("prefix", "", "call prefix")
	token                                   = flag.String("token", "", "bot token")
	clientID                                = ""
	textChannel  string                     = ""
	vcsession    *discordgo.VoiceConnection = nil
	speech_speed float64                    = 1.5
	speech_limit int                        = 100
	speech_lang  string                     = "auto"
	mut          sync.Mutex                 = sync.Mutex{}
)


func main() {
	//flag入手
	flag.Parse()
	fmt.Println("prefix       :", *prefix)
	fmt.Println("token        :", *token)

	//bot起動準備
	discord, err := discordgo.New()
	if err != nil {
		fmt.Println("Error logging")
	}

	//token入手
	discord.Token = "Bot " + *token

	//eventトリガー設定
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onVoiceStateUpdate)

	//起動
	if err = discord.Open(); err != nil {
		fmt.Println(err)
	}
	defer func() {
		if err := discord.Close(); err != nil {
			log.Println(err)
		}
	}()
	//起動メッセージ表示
	fmt.Println("Listening...")

	//bot停止対策
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}





//BOTの準備が終わったときにCall
func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID
	servers := 0
	for _, _ = range discord.State.Guilds {
	servers++
	}
	discord.UpdateStatus(0,*prefix+" help | "+strconv.Itoa(servers)+"個の鯖で稼働中")
}





//メッセージが送られたときにCall
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
  //一時変数
	GuildID := m.GuildID
	Guild_tmp,_ := discord.Guild(GuildID)
	Guild := Guild_tmp.Name
	ChannelID := m.ChannelID
	Channel,_ := discord.Channel(ChannelID)
	Message := m.ID
	Content := m.Content
	Author := m.Author.Username
	AuthorID := m.Author.ID

	//表示
	log.Print("Guild:\""+Guild+"\"  Channel:\""+Channel.Name+"\"  "+Author+": "+strings.Replace(Content,"\n"," \\n ",-1))

	//bot 読み上げ無し のチェック
	if m.Author.Bot || strings.HasPrefix(m.Content, ";") {
		return
	}

	switch {
		//TTS関連
		case Prefix(Content,"jjoin"):
			log.Print("join")
			if vcsession != nil {
				discord.ChannelMessageSend(ChannelID, "sorry this bot used")
				return
			}
			Join(ChannelID,discord,AuthorID,Message)
			return
		case Prefix(Content,"sspeed "):
			log.Print("speed")
			Speed(Content,discord,ChannelID,Message)
			return
		case Prefix(Content,"llang "):
			log.Print("lang")
			Lang(Content,discord,ChannelID,Message)
			return
		case Prefix(Content,"llimit "):
			log.Print("limit")
			Limit(Content,discord,ChannelID,Message)
			return
		case Prefix(Content,"wword "):
			log.Print("word")
			Word(Content,GuildID,discord,ChannelID,Message)
			return
		case Prefix(Content,"lleave"):
			log.Print("leave")
			if ChannelID == textChannel {
				Leave(discord,ChannelID,Message,true)
			}
			return
		//Poll関連
		case Prefix(Content,"poll "):
			Poll(Content,Author,discord,ChannelID,Message)
			return
		//Role関連
		case Prefix(Content,"role "):
			log.Print("role")
			return
		//help
		case Prefix(Content,"help"):
			Help(discord,ChannelID)
			return
	}

	//読み上げ
	if ChannelID == textChannel {
		replace := regexp.MustCompile(*prefix+" once ")
		text := replace.ReplaceAllString(Content, "")
		Speech(text,GuildID)
	}
}


func Prefix(message, check string) bool {
	return strings.HasPrefix(message, *prefix+" "+check)
}


func Join(ChannelID string, discord *discordgo.Session, AuthorID string, Message string) {
	if tmp,err := joinUserVoiceChannel(discord, AuthorID); err != nil {
		log.Println("missing join vc")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else {
		textChannel = ChannelID
		vcsession = tmp
		if err := discord.MessageReactionAdd(ChannelID,Message,"✅"); err!= nil {
			log.Println(err)
		}
		Speech("おはー","")
		return
	}
}

func Speech(text string,GuildID string) {

	data, _ :=  os.Open("./dic/"+GuildID+".txt")
	defer data.Close()
	scanner := bufio.NewScanner(data)
		for scanner.Scan(){
		tmp := scanner.Text()
		replace := regexp.MustCompile(`,.*`)
		from := replace.ReplaceAllString(tmp, "")
		replace = regexp.MustCompile(`.*,`)
		to := replace.ReplaceAllString(tmp, "")
		replace = regexp.MustCompile(from)
		text = replace.ReplaceAllString(text, to)
	}

	if regexp.MustCompile(`<a:|<:|<@|<#|<@&|http|` + "```").MatchString(text) {
		text = "message skip"
		return
	}

	lang := speech_lang
	if lang == "auto" {
		lang = "ja"
		if regexp.MustCompile(`^[a-zA-Z0-9\s.,]+$`).MatchString(text) {
			lang = "en"
		}
	}

	//text cut
	length := len(text)
	if length > speech_limit {
		text = string([]rune(text)[:speech_limit])
	}

	//改行停止
	if strings.Contains(text, "\n") {
		replace := regexp.MustCompile(`\n.*`)
		text = replace.ReplaceAllString(text, "")
	}

	//読み上げ待機
	mut.Lock()
	defer mut.Unlock()

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=32&client=tw-ob&q=%s&tl=%s", url.QueryEscape(text), lang)
	err := playAudioFile(vcsession,voiceURL)
	if err != nil {
		log.Printf("Error:%s  voiceURL:$s",err,voiceURL)
		return
	}
	return
}



func joinUserVoiceChannel(discord *discordgo.Session, userID string) (*discordgo.VoiceConnection, error) {
	vs := findUserVoiceState(discord, userID)
	return discord.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
}


func findUserVoiceState(discord *discordgo.Session, userid string) (*discordgo.VoiceState) {
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if vs.UserID == userid {
				return vs
			}
		}
	}
	return nil
}


func playAudioFile(voice *discordgo.VoiceConnection, filename string) error {
	if err := voice.Speaking(true); err != nil {
		return err
	}
	defer voice.Speaking(false)

	opts := dca.StdEncodeOptions
	opts.CompressionLevel = 0
	opts.RawOutput = true
	opts.Bitrate = 120
	opts.AudioFilter = fmt.Sprintf("atempo=%f", speech_speed)

	encodeSession, err := dca.EncodeFile(filename, opts)
	if err != nil {
		return err
	}

	done := make(chan error)
	stream := dca.NewStream(encodeSession, voice, done)
	ticker := time.NewTicker(time.Second)

	for {
		select {
		case err := <-done:
			if err != nil && err != io.EOF {
				return err
			}
			encodeSession.Truncate()
			return nil
		case <-ticker.C:
			playbackPosition := stream.PlaybackPosition()
			log.Println("Sending Now... : Playback:",playbackPosition)
		}
	}
}


func Speed(Content string, discord *discordgo.Session, ChannelID string, Message string) {
	tmp := strings.Replace(Content, *prefix+" sspeed ", "", 1)

	if tmp_speed, err := strconv.ParseFloat(tmp, 64); err != nil {
		log.Println("missing chenge string to float64")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else if tmp_speed < 0.5 || 100 < tmp_speed{
		log.Println("missing lowest or highest speed")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
	} else {
		speech_speed = tmp_speed
		if err := discord.MessageReactionAdd(ChannelID,Message,"🔊"); err!= nil {
			log.Println(err)
		}
		return
	}
}


func Lang(Content string, discord *discordgo.Session, ChannelID string, Message string) {
	tmp := strings.Replace(Content, *prefix+" llang ", "", 1)

	if tmp == "auto" {
		speech_lang = "auto"
		return
	}

	if _, err := language.Parse(tmp); err != nil {
		log.Println("missing chenge to unknown Language")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else {
		speech_lang = tmp
		if err := discord.MessageReactionAdd(ChannelID,Message,"🗣️"); err!= nil {
			log.Println(err)
		}
		return
	}
}


func Limit(Content string, discord *discordgo.Session, ChannelID string, Message string) {
	tmp := strings.Replace(Content, *prefix+" llimit ", "", 1)

	if tmp_limit, err := strconv.Atoi(tmp); err != nil {
		log.Println("missing chenge string to int")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else if tmp_limit <= 0 {
		log.Println("missing lowest limit")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else {
		speech_limit = tmp_limit
		if err := discord.MessageReactionAdd(ChannelID,Message,"🥺"); err!= nil {
			log.Println(err)
		}
		return
	}
}


func Word(Content string,GuildID string, discord *discordgo.Session, ChannelID string, Message string) {
	tmp := strings.Replace(Content, *prefix+" wword ", "", 1)

	if file, err := os.OpenFile("./dic/"+GuildID+".txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777); err != nil {
		//エラー処理
		log.Println("missing writing")
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
		return
	} else {
		defer file.Close()
		fmt.Fprintln(file, tmp)
		if err := discord.MessageReactionAdd(ChannelID,Message,"📄"); err!= nil {
			log.Println(err)
		}
		return
	}
}


func Leave(discord *discordgo.Session, ChannelID string, Message string, Reaction bool) {
	Speech("さいなら", "")
	
	if err := vcsession.Disconnect(); err != nil {
		log.Println("missing disconnect")
		if Reaction {
			if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
				log.Println(err)
			}
		}
		return
	} else {
		textChannel = ""
		vcsession = nil
		if Reaction {
			if err := discord.MessageReactionAdd(ChannelID,Message,"⛔"); err!= nil {
				log.Println(err)
			}
		}
		return
	}
}


func Poll(Content string, Author string, discord *discordgo.Session, ChannelID string, Message string) {
	//長さ確認
	replace := regexp.MustCompile(*prefix+" poll|,$")
	poll := replace.ReplaceAllString(Content, "")
	text := strings.Split(poll, ",")
	//Title+Questionだから-1
	length := len(text) - 1
	if length <= 20 {
		//embedとかreaction用のやつ
		alphabet := []string{"","🇦","🇧","🇨","🇩","🇪","🇫","🇬","🇭","🇮","🇯","🇰","🇱","🇲","🇳","🇴","🇵","🇶","🇷","🇸","🇹"}
		//embedのtmp作成
		embed := &discordgo.MessageEmbed{
			Type    : "rich",
			Title   : "",
			Description : "",
			Color   : 1000,
			Author  : &discordgo.MessageEmbedAuthor{Name:""},
		}
		//作成者表示
		embed.Author.Name = "create by @"+Author
		//Titleの設定
		embed.Title = text[0]
		//中身の設定
		Question := ""
		for i :=1 ; i < len(text); i++ {
			Question = Question+alphabet[i]+" : "+text[i]+"\n"
		}
		embed.Description = Question
		//送信
		message,err := discord.ChannelMessageSendEmbed(ChannelID,embed)
		if err!= nil {
			log.Println(err)
		}
		//リアクションと中身の設定
		for i :=1 ; i < len(text); i++ {
			Question = Question+alphabet[i]+text[i]+"\n"
			if err := discord.MessageReactionAdd(ChannelID,message.ID,alphabet[i]); err!= nil {
				log.Println(err)
			}
		}
	} else {
		if err := discord.MessageReactionAdd(ChannelID,Message,"❌"); err!= nil {
			log.Println(err)
		}
	}
}


func Help(discord *discordgo.Session, ChannelID string) {
	//embedのtmp作成
	embed := &discordgo.MessageEmbed{
		Type    : "rich",
		Title   : "BOT HELP",
		Description : "",
		Color   : 1000,
	}
	Text := "--TTS--\n"+
*prefix+" join :VCに参加します\n"+
*prefix+" speed <速度> : 読み上げ速度を変更します\n"+
*prefix+" lang <言語> : 読み上げ言語を変更します\n"+
*prefix+" limit <文字数> : 読み上げ文字数の上限を設定します\n"+
*prefix+" leave : VCから切断します\n"+
"--Poll--\n"+
*prefix+" poll <質問>,<回答1>,<回答2>... : 質問を作成します\n"
	embed.Description = Text
	//送信
	if _,err := discord.ChannelMessageSendEmbed(ChannelID,embed); err!= nil {
		log.Println(err)
	}
}





//VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {

	tmp := ","
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if v.ChannelID == vs.ChannelID && vs.UserID != clientID {
				return
			}
			tmp = tmp+vs.UserID+","
		}
	}
	if strings.Count(tmp, ",") == 2 {
		Leave(discord,"","",false)
	}
}