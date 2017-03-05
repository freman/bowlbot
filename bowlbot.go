/*
 *   bowlbot - Organise bowling games with a Telegram bot
 *   Copyright (c) 2017 Shannon Wynter.
 *
 *   This program is free software: you can redistribute it and/or modify
 *   it under the terms of the GNU General Public License as published by
 *   the Free Software Foundation, either version 3 of the License, or
 *   (at your option) any later version.
 *
 *   This program is distributed in the hope that it will be useful,
 *   but WITHOUT ANY WARRANTY; without even the implied warranty of
 *   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *   GNU General Public License for more details.
 *
 *   You should have received a copy of the GNU General Public License
 *   along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"gopkg.in/telegram-bot-api.v4"
)

const help = `The following commands are now available to you.
` + "```" + `
 /me    - to tell me of your desire to attend the event.
 /me+2  - as above but you're bringing two friends, how wonderful!
 /out   - to let me know you can no longer attend, how dreadful.
 /sub 1 - if it turns out that one of your friends cannot come.
 /count - to see how many people are coming, including their wonderful extras.
 /who   - for a list of who is coming.
` + "```"

const startEventMessagef = `Good news everyone!
%s has proposed we go bowling at %v on %v.
` + help

type bowlBot struct {
	bot    *tgbotapi.BotAPI
	db     *bowlingDB
	logger logrus.FieldLogger
	convos *convoState
}

func plural(count int, singular, multiple string) string {
	if count == 1 {
		return singular
	}
	return multiple
}

func (b *bowlBot) log() logrus.FieldLogger {
	if b.logger == nil {
		b.logger = logrus.New()
	}
	return b.logger
}

func (b *bowlBot) handleMe(args string, nextEvent *bowlingEvent, message *tgbotapi.Message) tgbotapi.MessageConfig {
	uid := message.From.ID
	from := message.From.String()
	args = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args), "+"))
	if len(args) > 0 {
		n, err := strconv.Atoi(args)
		if err != nil || n < 1 {
			return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("I beg your pardon %s, I'm not sure how to deal with that.", from))
		}
		fn, found := nextEvent.Attendees[uid]
		nextEvent.Attendees[uid] = n + 1
		nextEvent.save()
		if found {
			if fn > 1 {
				return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Ok %s, I have recorded that you're bringing %d people", from, n))
			}
			return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Excellent %s, the more the merrier!", from))
		}
		return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Brilliant %s, we look forward to seeing you and your %d extras", from, n))
	}

	nextEvent.Attendees[uid] = 1
	nextEvent.save()
	return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Awesome %s, we'll see you there", from))
}

func (b *bowlBot) handleSub(args string, nextEvent *bowlingEvent, message *tgbotapi.Message) tgbotapi.MessageConfig {
	uid := message.From.ID
	from := message.From.String()
	args = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args), "-"))
	if len(args) > 0 {
		n, err := strconv.Atoi(args)
		if err != nil || n < 1 {
			return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("I beg your pardon %s, I'm not sure how to deal with that.", from))
		}
		fn, found := nextEvent.Attendees[uid]
		if found {
			coming := fn - n
			if coming < 1 {
				coming = 1
			}
			nextEvent.Attendees[uid] = coming
			nextEvent.save()

			if coming > 1 {
				return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Ok %s, I have recorded that you're only bringing %d %s", from, coming-1, plural(coming-1, "person", "people")))
			}
			return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Ok %s, so it's just you.", from))
		}
		return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("But %s, you're not even coming!", from))
	}
	return tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("I beg your pardon %s, I'm not sure how to deal with that.", from))
}

func (b *bowlBot) handleCommand(group *bowlingGroup, message *tgbotapi.Message, edit bool) {
	command := message.Command()

	nextEvent := group.GetNextEvent()

	cmdIsBowling := strings.EqualFold(command, "bowling")
	eventIsNil := nextEvent == nil

	if eventIsNil && !cmdIsBowling {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Sorry %s, there is no game in the near future", message.From.String()))
		msg.DisableNotification = true
		b.bot.Send(msg)
		return
	}
	if !eventIsNil && cmdIsBowling {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, the next game is set for %v", message.From.String(), nextEvent.Time.Format("Monday 2")))
		msg.DisableNotification = true
		b.bot.Send(msg)
		return
	}
	if eventIsNil && cmdIsBowling {
		nextEvent := group.NewEvent()
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(startEventMessagef, message.From.String(), group.Time.Format("15:04"), nextEvent.Time.Weekday().String()))
		msg.ParseMode = tgbotapi.ModeMarkdown
		b.bot.Send(msg)
		return
	}

	var msg tgbotapi.MessageConfig

	switch command {
	case "help":
		msg = tgbotapi.NewMessage(message.Chat.ID, help)
		msg.ParseMode = tgbotapi.ModeMarkdown
		msg.DisableNotification = true
	case "me":
		msg = b.handleMe(message.CommandArguments(), nextEvent, message)
		msg.DisableNotification = true
	case "sub":
		msg = b.handleSub(message.CommandArguments(), nextEvent, message)
		msg.DisableNotification = true
	case "out":
		if _, found := nextEvent.Attendees[message.From.ID]; found {
			delete(nextEvent.Attendees, message.From.ID)
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Ok %s, sorry you can't make it", message.From.String()))
			nextEvent.save()
		} else {
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("But %s, you weren't actually listed as coming!", message.From.String()))
		}
		msg.DisableNotification = true
	case "count":
		c := 0
		for _, n := range nextEvent.Attendees {
			c += n
		}
		if c == 0 {
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, no-one has signed up yet", message.From.String()))
		} else {
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, there %s %d wonderful %s coming bowling", message.From.String(), plural(c, "is", "are"), c, plural(c, "person", "people")))
		}
		msg.DisableNotification = true
	case "who":
		coming := []string{}
		for id, n := range nextEvent.Attendees {
			user := b.db.loadUser(id)
			s := "   " + user.String()
			if n > 1 {
				s = fmt.Sprintf("%s and %d extra%s", s, n-1, plural(n-1, "", "s"))
			}
			coming = append(coming, s)
		}
		if len(coming) == 1 {
			who := strings.TrimSpace(coming[0])
			if strings.HasPrefix(who, message.From.String()) {
				who = "you" + strings.TrimPrefix(who, message.From.String())
			}
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, it looks like it's just %s", message.From.String(), who))
		} else if len(coming) == 0 {
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, no-one has signed up yet", message.From.String()))
		} else {
			msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey %s, the following people are coming\n%s", message.From.String(), strings.Join(coming, "\n")))
		}
		msg.DisableNotification = true
	case "cancel":
		group.NextEvent = ""
		group.save()
		msg = tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hey everyone, unfortunately bowling has been cancelled by %s", message.From.String()))
	default:
		if strings.HasPrefix(command, "me+") {
			msg = b.handleMe("+ "+strings.TrimPrefix(command, "me+"), nextEvent, message)
		}
		if strings.HasPrefix(command, "sub-") {
			msg = b.handleSub(strings.TrimPrefix(command, "sub-"), nextEvent, message)
		}
		msg.DisableNotification = true
	}

	b.bot.Send(msg)
}

func (b *bowlBot) handleMessage(message *tgbotapi.Message, edit bool) {
	if message.From != nil {
		b.db.saveUser(message.From)
	}
	if message.ForwardFrom != nil {
		b.db.saveUser(message.ForwardFrom)
	}
	if message.Chat.IsGroup() {
		if message.IsCommand() {
			chat := message.Chat

			group := b.db.getBowlingGroup(chat.ID)
			if !group.isConfigured() {
				group.Name = chat.Title
				group.save()
				b.askForConfiguration(message)
				return
			}

			b.handleCommand(group, message, edit)
			return
		}
		if message.ReplyToMessage != nil {
			b.convos.next(message)
		}
	}
}

func (b *bowlBot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		b.handleMessage(update.Message, false)
	}
	if update.EditedMessage != nil {
		b.handleMessage(update.EditedMessage, true)
	}
}

func main() {
	apikey := os.Getenv("API_KEY")
	callbackURL := os.Getenv("CALLBACK_URL")
	statedb := "state.db"
	listen := "127.0.0.1:8000"

	if tmp := os.Getenv("STATE_DB"); tmp != "" {
		statedb = tmp
	}
	if tmp := os.Getenv("LISTEN"); tmp != "" {
		listen = tmp
	}

	log := logrus.New()

	db, err := dbOpen(statedb)
	if err != nil {
		log.Fatal(err)
	}

	bot, err := tgbotapi.NewBotAPI(apikey)
	if err != nil {
		log.Fatal(err)
	}

	_, err = bot.SetWebhook(tgbotapi.NewWebhook(callbackURL + "/" + bot.Token))
	if err != nil {
		log.Fatal(err)
	}

	updates := bot.ListenForWebhook("/" + bot.Token)

	go http.ListenAndServe(listen, nil)

	bb := &bowlBot{bot: bot, db: db, convos: newConvoState()}

	for update := range updates {
		go bb.handleUpdate(update)
	}
}
