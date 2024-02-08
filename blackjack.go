package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aatomu/aatomlib/disgord"
	"github.com/bwmarrin/discordgo"
)

const (
	Master = "**Master**"
	Color  = 0x880088
)

type BlackjackSessions struct {
	sync.Mutex
	guilds []*blackjackSession
}

type blackjackSession struct {
	guildID     string
	channelID   string
	messageID   string
	phase       blackjackPhase
	acceptUsers map[string]bool
	users       map[string]*blackjackUser
	cards       []string
}

type blackjackPhase int

const (
	Wait blackjackPhase = iota
	BetTime
	HitTime
	Ended
)

type blackjackUser struct {
	coin    int
	bet     int
	cards   []string
	isBurst bool
	info    string
}

// Get Blackjack Session By GuildID
func (s *BlackjackSessions) Get(guildID string) *blackjackSession {
	for _, session := range s.guilds {
		if session.guildID != guildID {
			continue
		}
		return session
	}
	return nil
}

// Add Blackjack Session
func (s *BlackjackSessions) Add(session *blackjackSession) {
	s.Lock()
	defer s.Unlock()
	s.guilds = append(s.guilds, session)
}

// Delete Session
func (s *BlackjackSessions) Delete(guildID string) {
	s.Lock()
	defer s.Unlock()
	var newSessions []*blackjackSession
	for _, session := range s.guilds {
		if session.guildID == guildID {
			continue
		}
		newSessions = append(newSessions, session)
	}
	s.guilds = newSessions
}

func (s *BlackjackSessions) NewShuffledCards() (cards []string) {
	marks := []string{"♥", "♠", "♦", "♣"}
	numbers := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	for _, mark := range marks {
		for _, num := range numbers {
			cards = append(cards, mark+num)
		}
	}
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return
}

func (s *blackjackSession) UpdateMessage(discord *discordgo.Session) {
	// Embed Description Text
	var Text string
	// Sort User Name
	names := []string{}
	for k := range s.users {
		names = append(names, k)
	}
	sort.Strings(names)
	// Make User Information
	for _, name := range names {
		state := s.users[name]
		// Check Bet limit
		if state.bet > state.coin {
			state.bet = state.coin
		}
		// Make Text
		playerInfo := fmt.Sprintf("%s:\n  Coin:% 4d  Bet:% 4d\n  Card:", name, state.coin, state.bet)
		if name == Master && s.phase == HitTime {
			playerInfo += fmt.Sprintf("??,%s(% 1d)\n", state.cards[1], state.CardNum(1))
		} else {
			num := state.CardNum(0)
			playerInfo += fmt.Sprintf("%s(% 1d)\n", strings.Join(state.cards, ","), num)
			if num > 21 {
				state.isBurst = true
				playerInfo += "    Burst!\n"
			}
		}

		Text += playerInfo + state.info + "\n"
	}
	if Text != "" {
		Text = fmt.Sprintf("Player:\n```%s```", Text)
	}

	discord.ChannelMessageEditEmbed(s.channelID, s.messageID, &discordgo.MessageEmbed{
		Title:       "__**Blackjack**__",
		Color:       Color,
		Description: Text,
	})
}

func (s *blackjackSession) CardPop() (card string, ok bool) {
	remaining := len(s.cards)
	if remaining == 0 {
		return
	}

	card = s.cards[0]
	s.cards = s.cards[1:]
	ok = true
	return
}

func (s *blackjackSession) AcceptCount(userName string) (current int, required int) {
	_, ok := s.users[userName]
	if ok {
		s.acceptUsers[userName] = true
	}
	current = len(s.acceptUsers)
	var userCount float64
	for userName := range s.users {
		if userName != Master {
			userCount++
		}
	}
	required = int(math.Ceil((userCount / 3.0) * 2.0))
	return
}

func (u *blackjackUser) CardNum(offset int) (num int) {
	numbers := map[string]int{"A": 11, "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10, "J": 10, "Q": 10, "K": 10}
	ACardCount := 0
	for _, card := range u.cards[offset:] {
		card = strings.Join(strings.Split(card, "")[1:], "")
		num += numbers[card]
		if card == "A" {
			ACardCount++
		}
	}

	for num > 21 && ACardCount > 0 {
		num -= 10
		ACardCount--
	}
	return
}

func (s *blackjackSession) NewGame(res *disgord.InteractionResponse, discord *discordgo.Session, guildID, channelID string) {
	go res.Reply(nil)
	c, _ := discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "参加", Style: discordgo.PrimaryButton, CustomID: "blackjack-game-join"},
					discordgo.Button{Label: "退出", Style: discordgo.PrimaryButton, CustomID: "blackjack-game-leave"},
					discordgo.Button{Label: "ゲームを開始", Style: discordgo.SuccessButton, CustomID: "blackjack-game-start", Emoji: discordgo.ComponentEmoji{Name: ""}},
				},
			},
		},
	})

	session := &blackjackSession{
		guildID:     guildID,
		channelID:   c.ChannelID,
		messageID:   c.ID,
		phase:       Wait,
		acceptUsers: map[string]bool{},
		users:       map[string]*blackjackUser{},
		cards:       blackjack.NewShuffledCards(),
	}
	blackjack.Add(session)
	session.UpdateMessage(discord)
}

func (s *blackjackSession) GameJoin(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; ok {
		s.Failed(res, "Blackjack にすでに参加済みです")
		return
	}

	s.users[userName] = &blackjackUser{
		coin:    20,
		bet:     1,
		cards:   []string{},
		isBurst: false,
	}

	go res.Reply(nil)
	s.UpdateMessage(discord)
}

func (s *blackjackSession) GameLeave(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	delete(s.users, userName)
	delete(s.acceptUsers, userName)

	go res.Reply(nil)
	s.UpdateMessage(discord)
}

func (s *blackjackSession) GameStart(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	current, required := s.AcceptCount(userName)
	if current < required {
		text := fmt.Sprintf("Blackjack の開始まで `% 2d/% 2d`", current, required)
		go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: s.channelID,
			ID:      s.messageID,
			Content: &text,
		})
		go res.Reply(nil)
		return
	}

	s.phase = BetTime
	s.acceptUsers = map[string]bool{}
	go res.Reply(nil)

	text := "Blackjack を開始します"
	go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: s.channelID,
		ID:      s.messageID,
		Content: &text,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "ベット", Style: discordgo.SuccessButton, CustomID: "blackjack-bed-call"},
					discordgo.Button{Label: "ベットを締める", Style: discordgo.DangerButton, CustomID: "blackjack-bed-close"},
				},
			},
		},
	})
}

func (s *blackjackSession) BetCall(res *disgord.InteractionResponse, userName string) {
	user, ok := s.users[userName]
	if !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	go res.Window("ベッド額を指定", "blackjack-bed-input", disgord.ComponentNewLine().Input(discordgo.TextInput{
		Label:    fmt.Sprintf("ベッド額(Max:%d)", user.coin),
		Style:    discordgo.TextInputShort,
		Required: true,
		CustomID: "blackjack-bed-value",
	}))
}

func (s *blackjackSession) BetClose(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	current, required := s.AcceptCount(userName)
	if current < required {
		text := fmt.Sprintf("ベット を締めるまで `% 2d/% 2d`", current, required)
		go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: s.channelID,
			ID:      s.messageID,
			Content: &text,
		})
		go res.Reply(nil)
		return
	}

	s.phase = HitTime
	s.acceptUsers = map[string]bool{}
	go res.Reply(nil)

	s.users[Master] = &blackjackUser{
		coin:    9999,
		bet:     0,
		cards:   []string{},
		isBurst: false,
	}

	for _, user := range s.users {
		if user.coin == 0 {
			continue
		}
		card, ok := s.CardPop()
		if !ok {
			go res.Reply(&discordgo.InteractionResponseData{
				Content: "これ以上 カード を抽選できません",
			})
			break
		}
		user.cards = append(user.cards, card)

		card, ok = s.CardPop()
		if !ok {
			go res.Reply(&discordgo.InteractionResponseData{
				Content: "これ以上 カード を抽選できません",
			})
			break
		}
		user.cards = append(user.cards, card)
	}

	text := "ベット を締めました"
	go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: s.channelID,
		ID:      s.messageID,
		Content: &text,
		Components: disgord.ComponentParse(disgord.ComponentNewLine().
			Button(discordgo.Button{Label: "ヒット", Style: discordgo.SuccessButton, CustomID: "blackjack-card-hit"}).
			Button(discordgo.Button{Label: "カードを確定", Style: discordgo.DangerButton, CustomID: "blackjack-card-finish"}),
		),
	})

	s.UpdateMessage(discord)
}

func (s *blackjackSession) UpdateBetValue(res *disgord.InteractionResponse, discord *discordgo.Session, userName string, data discordgo.ModalSubmitInteractionData) {
	input := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput)
	n, err := strconv.Atoi(input.Value)
	if err != nil {
		return
	}

	user, ok := s.users[userName]
	if !ok {
		return
	}
	if n < 1 || n > user.coin {
		return
	}

	user.bet = n
	go res.Reply(nil)
	s.UpdateMessage(discord)
}

func (s *blackjackSession) CardHit(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	user, ok := s.users[userName]
	if !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	if len(s.cards) == 0 {
		s.Failed(res, "これ以上 カード を抽選できません")
		return
	}

	if user.isBurst {
		s.Failed(res, "バースト しています")
		return
	}

	if user.coin < 1 {
		s.Failed(res, "コインがありません")
	}

	go res.Reply(nil)

	card, ok := s.CardPop()
	if !ok {
		s.Failed(res, "これ以上 カード を抽選できません")
		return
	}

	user.cards = append(user.cards, card)
	if user.CardNum(0) > 21 {
		user.isBurst = true
	}

	s.UpdateMessage(discord)
}

func (s *blackjackSession) CardFinish(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	current, required := s.AcceptCount(userName)
	if current < required {
		text := fmt.Sprintf("カード を確定させるまで `% 2d/% 2d`", current, required)
		go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: s.channelID,
			ID:      s.messageID,
			Content: &text,
		})
		go res.Reply(nil)
		return
	}

	go res.Reply(nil)
	s.phase = Ended
	s.acceptUsers = map[string]bool{}

	master := s.users[Master]
	masterNum := master.CardNum(0)
	for masterNum < 17 {
		card, ok := s.CardPop()
		if !ok {
			break
		}
		master.cards = append(master.cards, card)
		masterNum = master.CardNum(0)
	}
	if master.CardNum(0) > 21 {
		master.isBurst = true
	}

	for userName, state := range s.users {
		if userName == Master {
			continue
		}

		userNum := state.CardNum(0)
		isJackpot := userNum == 21 && len(state.cards) == 2

		switch {
		case state.isBurst || (userNum < masterNum && !master.isBurst):
			state.info = fmt.Sprintf("    You Lose! -%d\n", state.bet)
			state.coin -= state.bet
		case userNum == masterNum:
			state.info = "    You are Draw! ±0\n"
		case (master.isBurst || userNum > masterNum) && isJackpot:
			betNum := int(math.Ceil(float64(state.bet) * 1.5))
			state.info = fmt.Sprintf("    You Win&Blackjack! +%d\n", betNum)
			state.coin += betNum
		case master.isBurst || userNum > masterNum:
			state.info = fmt.Sprintf("    You Win! +%d\n", state.bet)
			state.coin += state.bet
		}
	}

	text := "カード を確定しました"
	go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: s.channelID,
		ID:      s.messageID,
		Content: &text,
		Components: disgord.ComponentParse(
			disgord.ComponentNewLine().
				Button(discordgo.Button{Label: "ゲームを続ける", Style: discordgo.PrimaryButton, CustomID: "blackjack-game-continue"}).
				Button(discordgo.Button{Label: "ゲームを終了", Style: discordgo.DangerButton, CustomID: "blackjack-game-finish"}),
		),
	})
	s.UpdateMessage(discord)
}

func (s *blackjackSession) GameContinue(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	go res.Reply(nil)
	delete(s.users, Master)

	newUsers := map[string]*blackjackUser{}
	for name, state := range s.users {
		newUsers[name] = &blackjackUser{
			coin:    state.coin,
			bet:     1,
			cards:   []string{},
			isBurst: false,
			info:    "",
		}
	}

	guildID := s.guildID
	channelID := s.channelID
	messageID := s.messageID

	s = &blackjackSession{
		guildID:     guildID,
		channelID:   channelID,
		messageID:   messageID,
		phase:       Wait,
		acceptUsers: map[string]bool{},
		users:       newUsers,
		cards:       blackjack.NewShuffledCards(),
	}
	blackjack.Delete(guildID)
	blackjack.Add(s)

	text := "ゲーム を続けます"
	discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: s.channelID,
		ID:      s.messageID,
		Content: &text,
		Components: disgord.ComponentParse(
			disgord.ComponentNewLine().
				Button(discordgo.Button{Label: "参加", Style: discordgo.PrimaryButton, CustomID: "blackjack-game-join"}).
				Button(discordgo.Button{Label: "退出", Style: discordgo.PrimaryButton, CustomID: "blackjack-game-leave"}).
				Button(discordgo.Button{Label: "ゲームを開始", Style: discordgo.SuccessButton, CustomID: "blackjack-game-start"}),
		),
	})
	s.UpdateMessage(discord)
}

func (s *blackjackSession) GameFinish(res *disgord.InteractionResponse, discord *discordgo.Session, userName string) {
	if _, ok := s.users[userName]; !ok {
		s.Failed(res, "Blackjack に参加していません")
		return
	}

	current, required := s.AcceptCount(userName)
	if current < required {
		text := fmt.Sprintf("ゲーム を終了させるまで `% 2d/% 2d`", current, required)
		go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: s.channelID,
			ID:      s.messageID,
			Content: &text,
		})
		go res.Reply(nil)
		return
	}

	text := "ゲーム を終了しました"
	go discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: s.channelID,
		ID:      s.messageID,
		Content: &text,
		Components: disgord.ComponentParse(
			disgord.ComponentNewLine().
				Button(discordgo.Button{Label: "*", Style: discordgo.PrimaryButton, CustomID: "_"}),
		),
	})
}

func (s *blackjackSession) Failed(res *disgord.InteractionResponse, description string) {
	res.Reply(&discordgo.InteractionResponseData{
		Flags: discordgo.MessageFlagsEphemeral,
		Embeds: []*discordgo.MessageEmbed{
			{
				Title:       "Command Failed",
				Color:       embedColor,
				Description: description,
			},
		},
	})
}
