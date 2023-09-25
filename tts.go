package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aatomu/atomicgo/discordbot"
	"github.com/aatomu/atomicgo/files"
	"github.com/aatomu/atomicgo/utils"
	"github.com/aatomu/slashlib"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/language"
)

type ttsSessions struct {
	save   sync.Mutex
	guilds []*ttsSessionData
}

type ttsSessionData struct {
	guildID    string
	channelID  string
	vc         *discordgo.VoiceConnection
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

func (s *ttsSessions) Get(guildID string) *ttsSessionData {
	for _, session := range s.guilds {
		if session.guildID != guildID {
			continue
		}
		return session
	}
	return nil
}

func (s *ttsSessionData) IsJoined() bool {
	return s != nil
}

func (s *ttsSessions) Add(newSession *ttsSessionData) {
	s.save.Lock()
	defer s.save.Unlock()
	s.guilds = append(s.guilds, newSession)
}

func (s *ttsSessions) Delete(guildID string) {
	s.save.Lock()
	defer s.save.Unlock()
	var newSessions []*ttsSessionData
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

func (s *ttsSessionData) JoinVoice(res slashlib.InteractionResponse, guildID, channelID, userID string) {
	vcSession, err := discordbot.JoinUserVCchannel(res.Discord, userID, false, true)
	if utils.PrintError("Failed Join VoiceChat", err) {
		if res.Discord != nil {
			ttsSession.Failed(res, "ユーザーが VoiceChatに接続していない\nもしくは権限が不足しています")
		}
		return
	}

	session := &ttsSessionData{
		guildID:   guildID,
		channelID: channelID,
		vc:        vcSession,
		lead:      sync.Mutex{},
	}

	ttsSession.Add(session)

	session.Speech("BOT", "おはー")
	ttsSession.Success(res, "ハロー!")
}

func (s *ttsSessionData) LeaveVoice(res slashlib.InteractionResponse) {
	s.Speech("BOT", "さいなら")
	ttsSession.Success(res, "グッバイ!")
	time.Sleep(1 * time.Second)
	s.vc.Disconnect()

	ttsSession.Delete(s.guildID)
}

func (s *ttsSessionData) AutoLeave(discord *discordgo.Session, isJoin bool, userName string) {
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
		ttsSession.Delete(s.guildID)
	} else {
		// でなければ通知?
		if !s.updateInfo {
			return
		}
		if isJoin {
			s.Speech("BOT", fmt.Sprintf("%s join the voice", userName))
		} else { // 今 VCchannelIDがない
			s.Speech("BOT", fmt.Sprintf("%s left the voice", userName))
		}
	}
}

func (session *ttsSessionData) Speech(userID string, text string) {
	if session.CheckDic() {
		data, _ := os.Open("./dic/" + session.guildID + ".txt")
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
	text = regexp.MustCompile(`>>> `).ReplaceAllString(text, "")                  // quote
	text = regexp.MustCompile("```.*```").ReplaceAllString(text, "こーどぶろっく すーきっぷ") // codeblock
	// Word Decoration 2
	text = regexp.MustCompile(`~~(.+)~~`).ReplaceAllString(text, "$1")     // strikethrough
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

	settingData, err := ttsSession.Config(userID, UserSetting{})
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
	var end chan bool
	err = discordbot.PlayAudioFile(settingData.Speed, settingData.Pitch, session.vc, voiceURL, false, end)
	utils.PrintError("Failed play Audio \""+read+"\" ", err)
}

func (s *ttsSessionData) Dictionary(res slashlib.InteractionResponse, i slashlib.InteractionStruct) {
	//ファイルの指定
	fileName := "./dic/" + s.guildID + ".txt"
	//dicがあるか確認
	if !s.CheckDic() {
		ttsSession.Failed(res, "辞書の読み込みに失敗しました")
		return
	}

	textByte, _ := os.ReadFile(fileName)
	dic := string(textByte)

	//textをfrom toに
	from := i.CommandOptions["from"].StringValue()
	to := i.CommandOptions["to"].StringValue()

	// 禁止文字チェック
	if strings.Contains(from, ",") || strings.Contains(to, ",") {
		ttsSession.Failed(res, "使用できない文字が含まれています")
		return
	}

	//確認
	if strings.Contains(dic, from+",") {
		dic = utils.RegReplace(dic, "", "\n"+from+",.*")
	}
	dic = dic + from + "," + to + "\n"

	//書き込み
	err := files.WriteFileFlash(fileName, []byte(dic))
	if !utils.PrintError("Config Update Failed", err) {
		ttsSession.Failed(res, "辞書の書き込みに失敗しました")
		return
	}

	ttsSession.Success(res, "辞書を保存しました\n\""+from+"\" => \""+to+"\"")
}

func (s *ttsSessionData) ToggleUpdate(res slashlib.InteractionResponse) {
	s.updateInfo = !s.updateInfo

	ttsSession.Success(res, fmt.Sprintf("ボイスチャットの参加/退出の通知を %t に変更しました", s.updateInfo))
}

func (s *ttsSessionData) CheckDic() (ok bool) {
	// dic.txtがあるか
	if files.IsAccess("./dic/" + s.guildID + ".txt") {
		return true
	}

	//フォルダがあるか確認
	if !files.IsAccess("./dic") {
		//フォルダがなかったら作成
		err := files.Create("./dic", true)
		if utils.PrintError("Failed Create Dic", err) {
			return false
		}
	}

	//ファイル作成
	err := files.WriteFileFlash("./dic/"+s.guildID+".txt", []byte{})
	return !utils.PrintError("Failed create dictionary", err)
}

func (s *ttsSessions) Config(userID string, newConfig UserSetting) (result UserSetting, err error) {
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

	if !files.IsAccess(fileName) {
		if files.Create(fileName, false) != nil {
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
		fmt.Println(string(bytes))
		if err != nil {
			return dummy, fmt.Errorf("failed Marshal UserConfig")
		}
		//書き込み
		files.WriteFileFlash(fileName, bytes)
		log.Println("User userConfig Writed")
	}
	return
}

func (s *ttsSessions) UpdateConfig(res slashlib.InteractionResponse, i slashlib.InteractionStruct) {
	// 読み込み
	result, err := ttsSession.Config(i.UserID, UserSetting{})
	if utils.PrintError("Failed Get Config", err) {
		ttsSession.Failed(res, "読み上げ設定を読み込めませんでした")
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

	_, err = ttsSession.Config(i.UserID, result)
	if utils.PrintError("Failed Write Config", err) {
		ttsSession.Failed(res, "保存に失敗しました")
	}
	ttsSession.Success(res, "読み上げ設定を変更しました")
}

func (s *ttsSessions) Failed(res slashlib.InteractionResponse, description string) {
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

func (s *ttsSessions) Success(res slashlib.InteractionResponse, description string) {
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
