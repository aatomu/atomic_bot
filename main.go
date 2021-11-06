package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"log"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"golang.org/x/text/language"
)

type SessionData struct {
	guildID     string
	channelID   string
	vcsession   *discordgo.VoiceConnection
	speechSpeed float64
	speechLimit int
	speechLang  string
	mut         sync.Mutex
}

func GetByGuildID(guildID string) (*SessionData, error) {
	for _, s := range sessions {
		if s.guildID == guildID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("Cant find GuildID")
}

var (
	//変数定義
	prefix   = flag.String("prefix", "", "call prefix")
	token    = flag.String("token", "", "bot token")
	clientID = ""
	sessions = []*SessionData{}
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
	discord.AddHandler(onMessageReactionAdd)
	discord.AddHandler(onMessageReactionRemove)

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
	joinedVC := len(sessions)
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
		if err != nil {
			log.Println(err)
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
					guild, _ := discord.State.Guild(channel.GuildID)
					name := "User: " + strconv.Itoa(guild.MemberCount)
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//ロール数
				case strings.HasPrefix(channel.Name, "Role:"):
					guild, _ := discord.State.Guild(channel.GuildID)
					//@everyoneも入ってるから-1
					name := "Role: " + strconv.Itoa(len(guild.Roles)-1)
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//絵文字
				case strings.HasPrefix(channel.Name, "Emoji:"):
					guild, _ := discord.State.Guild(channel.GuildID)
					name := "Emoji: " + strconv.Itoa(len(guild.Emojis))
					if name != channel.Name {
						discord.ChannelEdit(channel.ID, name)
					}
					//チャンネル数
				case strings.HasPrefix(channel.Name, "Channel:"):
					guild, _ := discord.State.Guild(channel.GuildID)
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
	//一時変数
	guildID := m.GuildID
	guildData, err := discord.Guild(guildID)
	guildName := ""
	if err == nil {
		guildName = guildData.Name
	} else {
		guildName = "DirectMessage"
	}
	channelID := m.ChannelID
	channel, _ := discord.Channel(channelID)
	messageID := m.ID
	message := m.Content
	author := m.Author.Username
	authorNumber := m.Author.Discriminator
	authorID := m.Author.ID
	filesURL := ""
	if len(m.Attachments) > 0 {
		filesURL = "Files: \""
		for _, file := range m.Attachments {
			filesURL = filesURL + file.URL + ","
		}
		filesURL = filesURL + "\"  "
	}

	//表示
	log.Print("Guild:\"" + guildName + "\"  Channel:\"" + channel.Name + "\"  " + filesURL + "<" + author + "#" + authorNumber + ">: " + message)

	//bot 読み上げ無し のチェック
	if m.Author.Bot || strings.HasPrefix(m.Content, ";") {
		return
	}

	switch {
	//TTS関連
	case isPrefix(message, "join"):
		_, err := GetByGuildID(guildID)
		if err == nil {
			addReaction(discord, channelID, messageID, "❌")
			return
		}
		joinVoiceChat(channelID, guildID, discord, authorID, messageID)
		return
	case isPrefix(message, "get"):
		viewUserSetting(authorID, discord, channelID, messageID)
		return
	case isPrefix(message, "speed "):
		changeUserSpeed(authorID, message, discord, channelID, messageID)
		return
	case isPrefix(message, "pitch "):
		changeUserPitch(authorID, message, discord, channelID, messageID)
		return
	case isPrefix(message, "lang "):
		changeUserLang(authorID, message, discord, channelID, messageID)
		return
	case isPrefix(message, "limit "):
		session, err := GetByGuildID(guildID)
		if err != nil || session.channelID != channelID {
			addReaction(discord, channelID, messageID, "❌")
			return
		}
		changeSpeechLimit(session, message, discord, channelID, messageID)
		return
	case isPrefix(message, "word "):
		addWord(message, guildID, discord, channelID, messageID)
		return
	case isPrefix(message, "leave"):
		session, err := GetByGuildID(guildID)
		if err != nil || session.channelID != channelID {
			addReaction(discord, channelID, messageID, "❌")
			return
		}
		leaveVoiceChat(session, discord, channelID, messageID, true)
		return
		//Poll関連
	case isPrefix(message, "poll "):
		createPoll(message, author, discord, channelID, messageID)
		return
	//Role関連
	case isPrefix(message, "role "):
		if hasRole(discord, guildID, authorID, "RoleController") {
			crateRoleManager(message, author, discord, channelID, messageID)
			return
		}
		addReaction(discord, channelID, messageID, "❌")
		return
	//info
	case isPrefix(message, "info"):
		if hasRole(discord, guildID, authorID, "InfoController") {
			serverInfo(discord, guildID, channelID, messageID)
			return
		}
		addReaction(discord, channelID, messageID, "❌")
		return
		//help
	case isPrefix(message, "help"):
		sendHelp(discord, channelID)
		return
	}

	//読み上げ
	session, err := GetByGuildID(guildID)
	if err == nil && session.channelID == channelID {
		speechOnVoiceChat(authorID, session, message)
		return
	}

}

func isPrefix(message string, check string) bool {
	return strings.HasPrefix(message, *prefix+" "+check)
}

func hasRole(discord *discordgo.Session, guildID string, userID string, roleName string) bool {
	//ロール名チェック用変数
	guildRoleList, _ := discord.GuildRoles(guildID)
	userData, _ := discord.GuildMember(guildID, userID)
	roleData := userData.Roles
	//ロール名チェック
	for _, guildRole := range guildRoleList {
		for _, roleID := range roleData {
			if roleID == guildRole.ID && guildRole.Name == roleName {
				return true
			}
		}
	}
	return false
}

func joinVoiceChat(channelID string, guildID string, discord *discordgo.Session, authorID string, messageID string) {
	voiceConection, err := joinUserVoiceChannel(discord, authorID)
	if err != nil {
		log.Println("Error : Failed join vc")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	session := &SessionData{
		guildID:     guildID,
		channelID:   channelID,
		vcsession:   voiceConection,
		speechSpeed: 1.5,
		speechLimit: 50,
		speechLang:  "auto",
		mut:         sync.Mutex{},
	}
	sessions = append(sessions, session)
	addReaction(discord, channelID, messageID, "✅")
	speechOnVoiceChat("BOT", session, "おはー")
	return
}

func joinUserVoiceChannel(discord *discordgo.Session, userID string) (*discordgo.VoiceConnection, error) {
	vs := findUserVoiceState(discord, userID)
	return discord.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
}

func findUserVoiceState(discord *discordgo.Session, userid string) *discordgo.VoiceState {
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if vs.UserID == userid {
				return vs
			}
		}
	}
	return nil
}

func speechOnVoiceChat(userID string, session *SessionData, text string) {
	data, _ := os.Open("./dic/" + session.guildID + ".txt")

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

	lang, speed, pitch, err := userConfig(userID, "", 0, 0)
	if err != nil {
		log.Println(err)
	}

	if lang == "auto" {
		lang = "ja"
		if regexp.MustCompile(`^[a-zA-Z0-9\s.,]+$`).MatchString(text) {
			lang = "en"
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

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=100&client=tw-ob&q=%s&tl=%s", url.QueryEscape(read), lang)
	err = playAudioFile(speed, pitch, session, voiceURL)
	if err != nil {
		log.Printf("Error:%s voiceURL:%s", err, voiceURL)
		return
	}
	return
}

func playAudioFile(userSpeed float64, userPitch float64, session *SessionData, filename string) error {
	if err := session.vcsession.Speaking(true); err != nil {
		return err
	}
	defer session.vcsession.Speaking(false)

	opts := dca.StdEncodeOptions
	opts.CompressionLevel = 0
	opts.RawOutput = true
	opts.Bitrate = 120
	speed := userSpeed
	pitch := userPitch * 100
	opts.AudioFilter = fmt.Sprintf("aresample=24000,asetrate=24000*%f/100,atempo=100/%f*%f", pitch, pitch, speed)
	encodeSession, err := dca.EncodeFile(filename, opts)
	if err != nil {
		return err
	}

	done := make(chan error)
	stream := dca.NewStream(encodeSession, session.vcsession, done)
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
			log.Println("Sending Now... : Playback:", playbackPosition)
		}
	}
}

func viewUserSetting(userID string, discord *discordgo.Session, channelID string, messageID string) {
	lang, speed, pitch, err := userConfig(userID, "", 0, 0)
	if err != nil {
		log.Println("Faild get user config")
		addReaction(discord, channelID, messageID, "❌")
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
	if err != nil {
		log.Println("Faild get user data")
		addReaction(discord, channelID, messageID, "❌")
		return
	}
	embed.Title = "@" + userData.Username + "'s Speech Config"
	embedText := "Lang:\n" +
		lang + "\n" +
		"Speed:\n" +
		fmt.Sprint(speed) + "\n" +
		"Pitch:\n" +
		fmt.Sprint(pitch)
	embed.Description = embedText
	//送信
	if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
		log.Println("Faild send user config embed")
		log.Println(err)
	}
}

func changeUserSpeed(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	speedText := strings.Replace(message, *prefix+" speed ", "", 1)

	speed, err := strconv.ParseFloat(speedText, 64)
	if err != nil {
		log.Println("Failed change speed string to float64")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	if speed < 0.5 || 5 < speed {
		log.Println("Failed lowest or highest speed")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	_, _, _, err = userConfig(userID, "", speed, 0)
	if err != nil {
		log.Println(err)
		log.Println("Failed change speed")
		addReaction(discord, channelID, messageID, "❌")
		return
	}
	addReaction(discord, channelID, messageID, "🔊")
	return
}

func changeUserPitch(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	pitchText := strings.Replace(message, *prefix+" pitch ", "", 1)

	pitch, err := strconv.ParseFloat(pitchText, 64)
	if err != nil {
		log.Println("Failed change pitch string to float64")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	if pitch < 0.5 || 1.5 < pitch {
		log.Println("Failed lowest or highest pitch")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	_, _, _, err = userConfig(userID, "", 0, pitch)
	if err != nil {
		log.Println(err)
		log.Println("Failed change pitch")
		addReaction(discord, channelID, messageID, "❌")
		return
	}
	addReaction(discord, channelID, messageID, "🎶")
	return
}

func changeUserLang(userID string, message string, discord *discordgo.Session, channelID string, messageID string) {
	lang := strings.Replace(message, *prefix+" lang ", "", 1)

	if lang == "auto" {
		_, _, _, err := userConfig(userID, lang, 0, 0)
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}
		addReaction(discord, channelID, messageID, "🗣️")
		return
	}

	_, err := language.Parse(lang)
	if err != nil {
		log.Println("Failed change to unknown Language")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	_, _, _, err = userConfig(userID, lang, 0, 0)
	if err != nil {
		log.Println(err)
		log.Println("Failed change lang")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	addReaction(discord, channelID, messageID, "🗣️")
	return
}

func userConfig(userID string, userLang string, userSpeed float64, userPitch float64) (lang string, speed float64, pitch float64, returnErr error) {
	//変数定義
	lang = ""
	speed = 0.0
	pitch = 0.0
	returnErr = nil
	writeText := ""

	//BOTチェック
	if userID == "BOT" {
		lang = "ja"
		speed = 1.75
		pitch = 1
		returnErr = nil
		return
	}

	//ファイルパスの指定
	fileName := "./UserConfig.txt"

	text, err := readFile(fileName)
	if err != nil {
		log.Println(err)
		return
	}
	//UserIDからデータを入手
	reg := regexp.MustCompile(`^UserID:.* Lang:auto Speed:1 Pitch:1$`)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "UserID:"+userID) {
			fmt.Sscanf(line, "UserID:"+userID+" Lang:%s Speed:%f Pitch:%f", &lang, &speed, &pitch)
		} else {
			if line != "" && !reg.MatchString(line) {
				writeText = writeText + line + "\n"
			}
		}
	}

	//書き込みチェック用変数
	shouldWrite := false
	//上書き もしくはデータ作成
	//(userLang||userSpeed||userPitchが設定済み
	if userLang != "" || userSpeed != 0 || userPitch != 0 {
		shouldWrite = true
	}
	//(lang||speed||pitch)が入手できなかった時
	if lang == "" || speed == 0 || pitch == 0 {
		shouldWrite = true
	}
	if shouldWrite {
		//lang
		if lang == "" {
			lang = "auto"
		}
		if userLang != "" {
			lang = userLang
		}
		//speed
		if speed == 0 {
			speed = 1.0
		}
		if userSpeed != 0.0 {
			speed = userSpeed
		}
		//pitch
		if pitch == 0.0 {
			pitch = 1.0
		}
		if userPitch != 0 {
			pitch = userPitch
		}
		//最後に書き込むテキストを追加(Write==trueの時)
		writeText = writeText + "UserID:" + userID + " Lang:" + lang + " Speed:" + strconv.FormatFloat(speed, 'f', -1, 64) + " Pitch:" + strconv.FormatFloat(pitch, 'f', -1, 64)
		//書き込み
		err = ioutil.WriteFile(fileName, []byte(writeText), 0777)
		if err != nil {
			returnErr = err
			return
		}
		log.Println("User userConfig Writed")
	}
	return
}

func changeSpeechLimit(session *SessionData, message string, discord *discordgo.Session, channelID string, messageID string) {
	limitText := strings.Replace(message, *prefix+" limit ", "", 1)

	limit, err := strconv.Atoi(limitText)
	if err != nil {
		log.Println("Failed change speed string to int")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	if limit <= 0 || 50 < limit {
		log.Println("Failed lowest or highest limit")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	session.speechLimit = limit
	addReaction(discord, channelID, messageID, "🥺")
	return
}

func addWord(message string, guildID string, discord *discordgo.Session, channelID string, messageID string) {
	word := strings.Replace(message, *prefix+" word ", "", 1)

	if strings.Count(word, ",") != 1 {
		log.Println("unknown word")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	//フォルダがあるか確認
	_, err := os.Stat("./dic")
	//ファイルがなかったら作成
	if os.IsNotExist(err) {
		err = os.Mkdir("./dic", 0777)
		if err != nil {
			log.Println("Failed create directory")
			addReaction(discord, channelID, messageID, "❌")
			return
		}
	}

	//ファイルの指定
	fileName := "./dic/" + guildID + ".txt"
	//ファイルがあるか確認
	text, err := readFile(fileName)
	if err != nil {
		log.Println(err)
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	//textをにダブりがないかを確認&置換
	replace := regexp.MustCompile(`,.*`)
	check := replace.ReplaceAllString(word, "")
	if strings.Contains(text, check) {
		replace := regexp.MustCompile(`.*` + check + `,.*\n`)
		text = replace.ReplaceAllString(text, "")
	}
	text = text + word + "\n"
	//書き込み
	err = ioutil.WriteFile(fileName, []byte(text), 0777)
	if err != nil {
		log.Println("Failed write File")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	addReaction(discord, channelID, messageID, "📄")
	return
}

func leaveVoiceChat(session *SessionData, discord *discordgo.Session, channelID string, messageID string, reaction bool) {
	speechOnVoiceChat("BOT", session, "さいなら")

	if err := session.vcsession.Disconnect(); err != nil {
		log.Println("Failed disconnect")
		if reaction {
			addReaction(discord, channelID, messageID, "❌")
		}
		return
	} else {
		var ret []*SessionData
		for _, v := range sessions {
			if v.guildID == session.guildID {
				continue
			}
			ret = append(ret, v)
		}
		sessions = ret
		if reaction {
			addReaction(discord, channelID, messageID, "⛔")
		}
		return
	}
}

func createPoll(message string, author string, discord *discordgo.Session, channelID string, messageID string) {
	//複数?あるか確認
	if strings.Contains(message, ",") == false {
		log.Println("unknown word")
		addReaction(discord, channelID, messageID, "❌")
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
		if err != nil {
			log.Println(err)
		}
		//リアクションと中身の設定
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + text[i] + "\n"
			addReaction(discord, channelID, message.ID, alphabet[i])
		}
	} else {
		addReaction(discord, channelID, messageID, "❌")
	}
}

func crateRoleManager(message string, author string, discord *discordgo.Session, channelID string, messageID string) {
	//複数?あるか確認
	if strings.Contains(message, ",") == false {
		log.Println("unknown word")
		addReaction(discord, channelID, messageID, "❌")
		return
	}

	//roleが指定されてるか確認
	if strings.Contains(message, "<@&") == false {
		log.Println("unknown command")
		addReaction(discord, channelID, messageID, "❌")
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
		if err != nil {
			log.Println(err)
		}
		//リアクションと中身の設定
		for i := 1; i < len(text); i++ {
			Question = Question + alphabet[i] + text[i] + "\n"
			addReaction(discord, channelID, message.ID, alphabet[i])
		}
	} else {
		addReaction(discord, channelID, messageID, "❌")
	}
}

func serverInfo(discord *discordgo.Session, guildID string, channelID string, messageID string) {
	channels, _ := discord.GuildChannels(guildID)
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
				if err != nil {
					log.Println(err)
					addReaction(discord, channelID, messageID, "❌")
					return
				}
			}
		}
		//カテゴリー削除
		_, err := discord.ChannelDelete(categoryID)
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}
		addReaction(discord, channelID, messageID, "🛑")
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
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
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
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}

		//Roles
		createChannelData.Name = "Role: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}

		//Emoji
		createChannelData.Name = "Emoji: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}

		//Channel
		createChannelData.Name = "Channel: "
		_, err = discord.GuildChannelCreateComplex(guildID, createChannelData)
		if err != nil {
			log.Println(err)
			addReaction(discord, channelID, messageID, "❌")
			return
		}

		addReaction(discord, channelID, messageID, "📊")
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
		*prefix + " limit <1-50> : 読み上げ文字数の上限を設定します(Guild単位)\n" +
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
		log.Println("Faild send help embed")
		log.Println(err)
	}
}

//VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {

	//セッションがあるか確認
	session, err := GetByGuildID(v.GuildID)
	if err != nil {
		return
	}

	//VCに接続があるか確認
	if session.vcsession == nil || !session.vcsession.Ready {
		return
	}

	// ボイスチャンネルに誰かしらいたら return
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if session.vcsession.ChannelID == vs.ChannelID && vs.UserID != clientID {
				return
			}
		}
	}

	// ボイスチャンネルに誰もいなかったら Disconnect する
	leaveVoiceChat(session, discord, "", "", false)
}

//リアクション追加でCall
func onMessageReactionAdd(discord *discordgo.Session, reaction *discordgo.MessageReactionAdd) {
	//変数定義
	userID := reaction.UserID
	userData, _ := discord.User(userID)
	user := userData.Username
	emoji := reaction.Emoji.Name
	channelID := reaction.ChannelID
	channel, _ := discord.Channel(channelID)
	messageID := reaction.MessageID
	messageData, err := discord.ChannelMessage(channelID, messageID)
	message := ""
	if err == nil {
		message = messageData.Content
	}
	guildID := reaction.GuildID
	guildData, _ := discord.Guild(guildID)
	guild := guildData.Name

	//bot のチェック
	if userData.Bot {
		return
	}

	//改行あとを削除
	if strings.Contains(message, "\n") {
		replace := regexp.MustCompile(`\n.*`)
		message = replace.ReplaceAllString(message, "..")
	}

	//文字数を制限
	nowCount := 0
	logText := ""
	for _, word := range strings.Split(message, "") {
		if nowCount < 20 {
			logText = logText + word
			nowCount++
		}
	}

	//ログを表示
	log.Print("Guild:\"" + guild + "\"  Channel:\"" + channel.Name + "\"  Message:" + logText + "  User:" + user + "  Add:" + emoji)

	//embedがあるか確認
	if len(messageData.Embeds) == 0 {
		return
	}

	//Roleのやつか確認
	for _, embed := range messageData.Embeds {
		footerData := embed.Footer
		if footerData == nil || !strings.Contains(embed.Footer.Text, "RoleContoler") {
			return
		}
	}

	//複配列をstringに変換
	text := ""
	for _, embed := range messageData.Embeds {
		text = text + embed.Description
	}

	//stringを配列にして1個ずつ処理
	for _, embed := range strings.Split(text, "\n") {
		//ロール追加
		if strings.HasPrefix(embed, emoji) {
			replace := regexp.MustCompile(`[^0-9]`)
			roleID := replace.ReplaceAllString(embed, "")
			err := discord.GuildMemberRoleAdd(guildID, userID, roleID)
			//失敗時メッセージ出す
			if err != nil {
				log.Print(err)
				//embedのData作成
				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Description: "えらー : 追加できませんでした",
					Color:       1000,
				}
				//送信
				if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
					log.Println(err)
				}
			}
			return
		}
	}
}

//リアクション削除でCall
func onMessageReactionRemove(discord *discordgo.Session, reaction *discordgo.MessageReactionRemove) {
	//変数定義
	userID := reaction.UserID
	userData, _ := discord.User(userID)
	user := userData.Username
	emoji := reaction.Emoji.Name
	channelID := reaction.ChannelID
	channel, _ := discord.Channel(channelID)
	messageID := reaction.MessageID
	messageData, _ := discord.ChannelMessage(channelID, messageID)
	message := messageData.Content
	guildID := reaction.GuildID
	guildData, _ := discord.Guild(guildID)
	guild := guildData.Name

	//bot のチェック
	if userData.Bot {
		return
	}

	//改行あとを削除
	if strings.Contains(message, "\n") {
		replace := regexp.MustCompile(`\n.*`)
		message = replace.ReplaceAllString(message, "..")
	}

	//文字数を制限
	nowCount := 0
	logText := ""
	for _, word := range strings.Split(message, "") {
		if nowCount < 20 {
			logText = logText + word
			nowCount++
		}
	}

	//ログを表示
	log.Print("Guild:\"" + guild + "\"  Channel:\"" + channel.Name + "\"  Message:" + logText + "  User:" + user + "  Remove:" + emoji)

	//embedがあるか確認
	if len(messageData.Embeds) == 0 {
		return
	}

	//Roleのやつか確認
	for _, embed := range messageData.Embeds {
		footerData := embed.Footer
		if footerData == nil || !strings.Contains(embed.Footer.Text, "RoleContoler") {
			return
		}
	}

	//複配列をstringに変換
	text := ""
	for _, embed := range messageData.Embeds {
		text = text + embed.Description
	}

	//stringを配列にして1個ずつ処理
	for _, embed := range strings.Split(text, "\n") {
		//ロール追加
		if strings.HasPrefix(embed, emoji) {
			replace := regexp.MustCompile(`[^0-9]`)
			roleID := replace.ReplaceAllString(embed, "")
			err := discord.GuildMemberRoleRemove(guildID, userID, roleID)
			//失敗時メッセージ出す
			if err != nil {
				log.Print(err)
				//embedのData作成
				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Description: "えらー : 削除できませんでした",
					Color:       1000,
				}
				//送信
				if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
					log.Println(err)
				}
			}
			return
		}
	}
}

//リアクション追加用
func addReaction(discord *discordgo.Session, channelID string, messageID string, reaction string) {
	err := discord.MessageReactionAdd(channelID, messageID, reaction)
	if err != nil {
		log.Print("Error: addReaction Failed")
		log.Println(err)
	}
	return
}

//ファイル読み込み
func readFile(filePath string) (text string, returnErr error) {
	//ファイルがあるか確認
	_, err := os.Stat(filePath)
	//ファイルがなかったら作成
	if os.IsNotExist(err) {
		_, err = os.Create(filePath)
		if err != nil {
			log.Println(err)
			returnErr = fmt.Errorf("missing crate file")
			return
		}
	}

	//読み込み
	byteText, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Println(err)
		returnErr = fmt.Errorf("missing read file")
		return
	}

	//[]byteをstringに
	text = string(byteText)
	return
}
